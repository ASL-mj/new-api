package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TaskSubmissionPrepared    = "prepared"
	TaskSubmissionAccepted    = "accepted"
	TaskSubmissionApplied     = "applied"
	TaskSubmissionUnknown     = "unknown"
	TaskSubmissionPendingCode = "task_submission_pending"
	TaskSubmissionUnknownCode = "h3_submit_outcome_unknown"
)

// Submission accounting is frozen before the request leaves the process. It
// contains no credentials and survives both session loss and LOG_DB outages.
type taskSubmissionAccounting struct {
	UserID         int            `json:"user_id"`
	TokenID        int            `json:"token_id"`
	ChannelID      int            `json:"channel_id"`
	SubscriptionID int            `json:"subscription_id"`
	BillingSource  string         `json:"billing_source"`
	FundingPre     int            `json:"funding_pre"`
	TokenPre       int            `json:"token_pre"`
	ActualQuota    int            `json:"actual_quota"`
	StandardQuota  int            `json:"standard_quota"`
	UsageAt        int64          `json:"usage_at"`
	Username       string         `json:"username"`
	TokenName      string         `json:"token_name"`
	ModelName      string         `json:"model_name"`
	Group          string         `json:"group"`
	Other          map[string]any `json:"other"`
}

// PrepareTaskSubmissionAccounting must succeed before DoRequest. Existing
// billing precharges are synchronous only when batch quota writes are disabled.
func PrepareTaskSubmissionAccounting(ctx context.Context, task *model.Task, info *relaycommon.RelayInfo) error {
	if task == nil || task.ID == 0 || info == nil || common.BatchUpdateEnabled {
		return errors.New("durable task submission requires a persisted task and synchronous quota writes")
	}
	a := taskSubmissionAccounting{UserID: task.UserId, TokenID: task.PrivateData.TokenId, ChannelID: task.ChannelId,
		SubscriptionID: task.PrivateData.SubscriptionId, BillingSource: task.PrivateData.BillingSource,
		ActualQuota: info.PriceData.Quota, StandardQuota: standardTaskQuotaEstimate(info), UsageAt: time.Now().Unix(),
		ModelName: taskModelName(task), Group: task.Group, Other: taskBillingOther(task)}
	if info.IsPlayground {
		a.TokenID = 0
	}
	if info.Billing != nil {
		session, ok := info.Billing.(*BillingSession)
		if !ok {
			return errors.New("unsupported task billing session")
		}
		session.mu.Lock()
		a.FundingPre, a.TokenPre = session.preConsumedQuota, session.tokenConsumed
		session.mu.Unlock()
	}
	var user model.User
	if err := model.DB.WithContext(ctx).Select("id", "username").First(&user, a.UserID).Error; err != nil {
		return err
	}
	a.Username = user.Username
	if a.TokenID > 0 {
		var token model.Token
		if err := model.DB.WithContext(ctx).Select("id", "name").Where("id = ? AND user_id = ?", a.TokenID, a.UserID).First(&token).Error; err != nil {
			return err
		}
		a.TokenName = token.Name
	}
	payload, err := common.Marshal(a)
	if err != nil {
		return err
	}
	result := model.DB.WithContext(ctx).Model(&model.Task{}).
		Where("id = ? AND status = ? AND submission_settled = ? AND submission_state IN ?", task.ID, model.TaskStatusNotStart, false, []string{"", TaskSubmissionPrepared}).
		Updates(map[string]any{"submission_state": TaskSubmissionPrepared, "submission_payload": string(payload)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("task submission is already owned")
	}
	return nil
}

// CompleteTaskSubmission first journals the accepted upstream identity while
// NOT_START, then atomically commits accounting and polling readiness. Recovery
// needs no in-memory BillingSession and never resubmits an upstream request.
func CompleteTaskSubmission(ctx context.Context, task *model.Task, info *relaycommon.RelayInfo, upstreamID string, data []byte) error {
	if task == nil || task.ID == 0 || info == nil || upstreamID == "" || info.PriceData.Quota < 0 {
		return errors.New("invalid task submission completion")
	}
	if err := stageTaskSubmission(ctx, task.ID, upstreamID, data, info); err != nil {
		return err
	}
	if err := completeTaskSubmissionByID(ctx, model.DB, task.ID); err != nil {
		return err
	}
	if session, ok := info.Billing.(*BillingSession); ok {
		session.mu.Lock()
		session.settled, session.fundingSettled = true, true
		session.mu.Unlock()
	}
	// A log/cache outage never turns a committed submission into a failed request.
	if err := RetryTaskSubmissionEffects(ctx, 100); err != nil {
		common.SysLog("task submission effects remain pending")
	}
	return model.DB.WithContext(ctx).First(task, task.ID).Error
}

// completeTaskSubmissionByID commits the provider binding, the initial
// channel/request accounting and the polling gate in one database transaction.
// Wallet/token pre-consumption has already happened in BillingSession; this
// transaction records the corresponding used-quota fact exactly once.
func completeTaskSubmissionByID(ctx context.Context, db *gorm.DB, id int64) error {
	if db == nil {
		return errors.New("database not initialized")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, id).Error; err != nil {
			return err
		}
		if task.SubmissionSettled && task.SubmissionState == TaskSubmissionApplied {
			return nil
		}
		if task.SubmissionState != TaskSubmissionAccepted || task.PrivateData.UpstreamTaskID == "" {
			return errors.New("task submission is not accepted")
		}
		var a taskSubmissionAccounting
		if err := common.Unmarshal([]byte(task.SubmissionPayload), &a); err != nil {
			return fmt.Errorf("invalid task submission accounting: %w", err)
		}
		if a.UserID <= 0 || a.ActualQuota < 0 || a.StandardQuota < 0 {
			return errors.New("invalid task submission accounting facts")
		}
		if a.ChannelID > 0 {
			if _, err := model.ApplyChannelUsageDeltaTx(tx, model.ChannelUsageDeltaParams{
				ChannelID:             task.ChannelId,
				KeyFingerprint:        task.PrivateData.ChannelKeyFingerprint,
				KeyIndex:              task.PrivateData.ChannelKeyIndex,
				HasKeyIdentity:        task.PrivateData.ChannelKeyFingerprint != "",
				QuotaDelta:            a.ActualQuota,
				StandardQuotaDelta:    int64(a.StandardQuota),
				StandardQuotaExplicit: true,
				RequestDelta:          1,
				Now:                   time.Unix(a.UsageAt, 0),
			}); err != nil {
				return err
			}
		}
		userUpdate := tx.Model(&model.User{}).Where("id = ?", a.UserID).Updates(map[string]any{
			"used_quota":    gorm.Expr("used_quota + ?", a.ActualQuota),
			"request_count": gorm.Expr("request_count + 1"),
		})
		if userUpdate.Error != nil {
			return userUpdate.Error
		}
		if userUpdate.RowsAffected != 1 {
			return errors.New("task submission user is missing")
		}
		privateData := task.PrivateData
		privateData.ChannelUsageRecordedAt = a.UsageAt
		privateData.ChannelStandardQuota = a.StandardQuota
		if err := tx.Model(&model.Task{}).Where("id = ? AND submission_state = ?", id, TaskSubmissionAccepted).Updates(map[string]any{
			"status":                model.TaskStatusSubmitted,
			"progress":              "0%",
			"quota":                 a.ActualQuota,
			"submission_state":      TaskSubmissionApplied,
			"submission_settled":    true,
			"submission_effects_at": 0,
			"submission_next_at":    0,
			"submission_attempts":   0,
			"next_poll_at":          time.Now().Unix(),
			"private_data":          privateData,
		}).Error; err != nil {
			return err
		}
		return nil
	})
}

// RetryTaskSubmissionEffects writes the initial usage log after the accounting
// transaction. The unique TaskSubmissionID makes crash/replay idempotent.
func RetryTaskSubmissionEffects(ctx context.Context, limit int) error {
	if model.DB == nil {
		return errors.New("database not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	var tasks []model.Task
	if err := model.DB.WithContext(ctx).Where("submission_state = ? AND submission_settled = ? AND submission_effects_at = 0 AND (submission_next_at = 0 OR submission_next_at <= ?)", TaskSubmissionApplied, true, time.Now().Unix()).Order("id").Limit(limit).Find(&tasks).Error; err != nil {
		return err
	}
	if !common.LogConsumeEnabled {
		return model.DB.Model(&model.Task{}).Where("id IN ?", taskIDs(tasks)).Updates(map[string]any{"submission_effects_at": time.Now().Unix()}).Error
	}
	if model.LOG_DB == nil {
		return errors.New("task submission log database is not initialized")
	}
	var firstErr error
	for i := range tasks {
		task := &tasks[i]
		var a taskSubmissionAccounting
		if err := common.Unmarshal([]byte(task.SubmissionPayload), &a); err != nil {
			firstErr = firstError(firstErr, err)
			continue
		}
		other := cloneAnyMap(a.Other)
		other["task_id"] = task.TaskID
		other["task_billing"] = map[string]any{"task_id": task.TaskID, "pre_consumed_quota": a.ActualQuota, "actual_quota": a.ActualQuota, "quota_delta": 0, "settlement_status": "applied", "settlement_kind": "settle"}
		submissionID := task.ID
		entry := &model.Log{TaskSubmissionID: &submissionID, UserId: a.UserID, CreatedAt: a.UsageAt, Type: model.LogTypeConsume, Username: a.Username, TokenName: a.TokenName, ModelName: a.ModelName, Quota: a.ActualQuota, ChannelId: a.ChannelID, TokenId: a.TokenID, Group: a.Group, Other: common.MapToJsonStr(other)}
		if err := model.LOG_DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(entry).Error; err != nil {
			firstErr = firstError(firstErr, err)
			_ = model.DB.Model(task).Updates(map[string]any{"submission_attempts": gorm.Expr("submission_attempts + 1"), "submission_next_at": time.Now().Unix() + 15}).Error
			continue
		}
		if err := model.DB.Model(task).Where("submission_effects_at = 0").Updates(map[string]any{"submission_effects_at": time.Now().Unix(), "submission_attempts": gorm.Expr("submission_attempts + 1")}).Error; err != nil {
			firstErr = firstError(firstErr, err)
		}
	}
	return firstErr
}

// RetryTaskSubmissionAccounting completes the durable phase for tasks where
// the upstream ID was staged but the process stopped before the local commit.
// It never sends another provider request and therefore cannot duplicate a
// generation.
func RetryTaskSubmissionAccounting(ctx context.Context, limit int) error {
	if model.DB == nil {
		return errors.New("database not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	var tasks []model.Task
	if err := model.DB.WithContext(ctx).Where("submission_state = ? AND submission_settled = ? AND submission_next_at <= ?", TaskSubmissionAccepted, false, time.Now().Unix()).Order("id").Limit(limit).Find(&tasks).Error; err != nil {
		return err
	}
	var firstErr error
	for i := range tasks {
		if err := completeTaskSubmissionByID(ctx, model.DB, tasks[i].ID); err != nil {
			firstErr = firstError(firstErr, err)
			_ = model.DB.Model(&tasks[i]).Updates(map[string]any{"submission_attempts": gorm.Expr("submission_attempts + 1"), "submission_next_at": time.Now().Unix() + 15}).Error
		}
	}
	return firstErr
}

func taskIDs(tasks []model.Task) []int64 {
	ids := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func cloneAnyMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func firstError(current, next error) error {
	if current != nil {
		return current
	}
	return next
}

func stageTaskSubmission(ctx context.Context, id int64, upstreamID string, data []byte, info *relaycommon.RelayInfo) error {
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&model.Task{}).Where("id = ? AND status = ? AND submission_state = ?", id, model.TaskStatusNotStart, TaskSubmissionPrepared).
			Update("submission_state", TaskSubmissionAccepted)
		if claim.Error != nil {
			return claim.Error
		}
		var current model.Task
		if err := tx.First(&current, id).Error; err != nil {
			return err
		}
		if claim.RowsAffected == 0 {
			if (current.SubmissionState == TaskSubmissionAccepted || current.SubmissionSettled) && current.PrivateData.UpstreamTaskID == upstreamID {
				return nil
			}
			return errors.New("conflicting task submission completion")
		}
		var a taskSubmissionAccounting
		if err := common.Unmarshal([]byte(current.SubmissionPayload), &a); err != nil {
			return err
		}
		a.ActualQuota = info.PriceData.Quota
		a.StandardQuota = standardTaskQuotaEstimate(info)
		payload, err := common.Marshal(a)
		if err != nil {
			return err
		}
		privateData := current.PrivateData
		privateData.UpstreamTaskID = upstreamID
		return tx.Model(&current).Updates(map[string]any{"private_data": privateData, "data": data, "submission_payload": string(payload)}).Error
	})
}

// MarkTaskSubmissionUnknown records only a fixed diagnostic, never provider
// bodies, transport errors, signed URLs, or credentials.
func MarkTaskSubmissionUnknown(ctx context.Context, id int64) error {
	return model.DB.WithContext(ctx).Model(&model.Task{}).
		Where("id = ? AND status = ? AND submission_state = ?", id, model.TaskStatusNotStart, TaskSubmissionPrepared).
		Updates(map[string]any{"submission_state": TaskSubmissionUnknown, "last_poll_error_class": "internal",
			"last_poll_error_message": "Submission outcome unknown; reconcile upstream before retry or refund."}).Error
}

func TaskSubmissionRetainsFunding(taskErrCode string) bool {
	return taskErrCode == TaskSubmissionUnknownCode || taskErrCode == TaskSubmissionPendingCode
}
