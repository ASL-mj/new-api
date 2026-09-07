package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The outbox carries identifiers and public billing facts, never credentials.
type taskSettlementEffects struct {
	TaskID    string         `json:"task_id"`
	UserID    int            `json:"user_id"`
	TokenID   int            `json:"token_id"`
	ChannelID int            `json:"channel_id"`
	ModelName string         `json:"model_name"`
	Group     string         `json:"group"`
	Other     map[string]any `json:"other"`
}

// SettleTaskTerminal commits terminal state, funds and the outbox together.
// A lost lease or an already terminal task never applies the same delta twice.
func SettleTaskTerminal(ctx context.Context, task *model.Task, fromStatus model.TaskStatus, actualQuota, standardActualQuota int, totalTokens int64, reasonClass string) error {
	if task == nil || task.ID == 0 || actualQuota < 0 || standardActualQuota < 0 || totalTokens < 0 {
		return errors.New("invalid terminal settlement")
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		return errors.New("terminal task status required")
	}
	if fromStatus == model.TaskStatusSuccess || fromStatus == model.TaskStatusFailure || task.SettlementStatus == model.TaskSettlementStatusLegacy {
		return nil
	}
	now := time.Now().Unix()
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire a row write lock with a lease-guarded UPDATE, including on SQLite.
		claim := tx.Model(&model.Task{}).
			Where("id = ? AND status = ? AND last_poll_at = ? AND next_poll_at = ? AND settlement_status <> ?", task.ID, fromStatus, task.LastPollAt, task.NextPollAt, model.TaskSettlementStatusLegacy).
			Update("settlement_status", model.TaskSettlementStateApplying)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			return nil
		}
		var current model.Task
		if err := tx.First(&current, task.ID).Error; err != nil {
			return err
		}
		pre, actual, standard := current.Quota, actualQuota, standardActualQuota
		kind := model.TaskSettlementKindSettle
		if task.Status == model.TaskStatusFailure {
			kind, actual, standard = model.TaskSettlementKindRefund, 0, 0
		}
		delta := actual - pre
		standardDelta := standard - current.PrivateData.ChannelStandardQuota
		other := taskBillingOther(&current)
		other["task_id"] = current.TaskID
		other["task_billing"] = map[string]any{"task_id": current.TaskID, "pre_consumed_quota": pre, "actual_quota": actual, "quota_delta": delta, "settlement_kind": kind, "settlement_status": "applied", "settled_at": now}
		effects := taskSettlementEffects{TaskID: current.TaskID, UserID: current.UserId, TokenID: current.PrivateData.TokenId, ChannelID: current.ChannelId, ModelName: taskModelName(&current), Group: current.Group, Other: other}
		payload, err := common.Marshal(effects)
		if err != nil {
			return err
		}
		snapshot, err := common.Marshal(current.PrivateData.BillingContext)
		if err != nil {
			return err
		}
		settlement := &model.TaskSettlement{TaskID: current.ID, Kind: kind, State: model.TaskSettlementStateApplying, PreConsumedQuota: pre, ActualQuota: &actual, QuotaDelta: delta, StandardQuotaDelta: standardDelta, ReasonClass: reasonClass, ReasonMessage: task.FailReason, PricingSnapshot: snapshot, EffectsPayload: string(payload), AttemptCount: 1}
		if kind == model.TaskSettlementKindRefund {
			settlement.ActualQuota = nil
		}
		if err := tx.Create(settlement).Error; err != nil {
			return err
		}
		if err := applyTaskFundingTx(tx, &current, delta); err != nil {
			return err
		}
		if current.PrivateData.TokenId > 0 && delta != 0 {
			if err := tx.Model(&model.Token{}).Where("id = ? AND user_id = ?", current.PrivateData.TokenId, current.UserId).
				Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", delta), "used_quota": gorm.Expr("CASE WHEN used_quota + ? < 0 THEN 0 ELSE used_quota + ? END", delta, delta)}).Error; err != nil {
				return err
			}
		}
		if delta != 0 {
			if err := tx.Model(&model.User{}).Where("id = ?", current.UserId).
				Update("used_quota", gorm.Expr("CASE WHEN used_quota + ? < 0 THEN 0 ELSE used_quota + ? END", delta, delta)).Error; err != nil {
				return err
			}
		}
		if current.ChannelId > 0 {
			result, err := model.ApplyChannelUsageDeltaTx(tx, model.ChannelUsageDeltaParams{ChannelID: current.ChannelId, KeyFingerprint: current.PrivateData.ChannelKeyFingerprint, KeyIndex: current.PrivateData.ChannelKeyIndex, HasKeyIdentity: current.PrivateData.ChannelKeyFingerprint != "", QuotaDelta: delta, StandardQuotaDelta: int64(standardDelta), StandardQuotaExplicit: true, TokenUsedDelta: totalTokens, Now: taskChannelUsageTime(&current)})
			if err != nil {
				return err
			}
			if delta != 0 || standardDelta != 0 || totalTokens != 0 {
				if err := tx.Model(&model.Ability{}).Where("channel_id = ?", current.ChannelId).Update("enabled", result.Channel.Status == common.ChannelStatusEnabled).Error; err != nil {
					return err
				}
			}
		}
		privateData := current.PrivateData
		privateData.ResultURL = task.PrivateData.ResultURL
		if err := tx.Model(&model.Task{}).Where("id = ?", current.ID).Updates(map[string]any{
			"status": task.Status, "progress": "100%", "quota": actual, "settlement_status": "applied", "settled_at": now, "next_poll_at": 0,
			"start_time": task.StartTime, "finish_time": task.FinishTime, "fail_reason": task.FailReason, "data": task.Data, "private_data": privateData,
			"poll_failures": task.PollFailures, "last_poll_error_class": task.LastPollErrorClass, "last_poll_error_message": task.LastPollErrorMessage,
		}).Error; err != nil {
			return err
		}
		return tx.Model(settlement).Updates(map[string]any{"state": model.TaskSettlementStateApplied, "applied_at": now}).Error
	})
	if err != nil {
		return err
	}
	// Failure here leaves the durable outbox pending, not partially applied funds.
	return RetryTaskSettlementEffects(ctx, 100)
}

func applyTaskFundingTx(tx *gorm.DB, task *model.Task, delta int) error {
	if delta == 0 {
		return nil
	}
	var result *gorm.DB
	if taskIsSubscription(task) {
		result = tx.Model(&model.UserSubscription{}).Where("id = ? AND user_id = ?", task.PrivateData.SubscriptionId, task.UserId).
			Update("amount_used", gorm.Expr("CASE WHEN amount_used + ? < 0 THEN 0 ELSE amount_used + ? END", delta, delta))
	} else {
		result = tx.Model(&model.User{}).Where("id = ?", task.UserId).Update("quota", gorm.Expr("quota - ?", delta))
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("task funding source missing")
	}
	return nil
}

// RetryTaskSettlementEffects retries independent effects. Unique log ownership
// also covers a crash between writing LOG_DB and acknowledging this outbox.
func RetryTaskSettlementEffects(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var rows []model.TaskSettlement
	now := time.Now().Unix()
	if err := model.DB.WithContext(ctx).Where("state = ? AND effects_applied_at = 0 AND effects_next_at <= ?", model.TaskSettlementStateApplied, now).Order("id").Limit(limit).Find(&rows).Error; err != nil {
		return err
	}
	var firstErr error
	for i := range rows {
		row := &rows[i]
		if err := applyTaskSettlementEffects(ctx, row); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			_ = model.DB.Model(row).Updates(map[string]any{"effects_next_at": now + 15, "effects_attempts": gorm.Expr("effects_attempts + 1")}).Error
			continue
		}
		if err := model.DB.Model(row).Updates(map[string]any{"effects_applied_at": now, "effects_attempts": gorm.Expr("effects_attempts + 1")}).Error; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func applyTaskSettlementEffects(ctx context.Context, row *model.TaskSettlement) error {
	var e taskSettlementEffects
	if err := common.Unmarshal([]byte(row.EffectsPayload), &e); err != nil {
		return err
	}
	if err := model.InvalidateUserCache(e.UserID); err != nil {
		return err
	}
	if err := model.InvalidateUserTokensCache(e.UserID); err != nil {
		return err
	}
	if e.ChannelID > 0 {
		if err := model.RefreshTaskChannelCache(e.ChannelID); err != nil {
			return err
		}
	}
	if model.LOG_DB == nil {
		return fmt.Errorf("task settlement log database is not initialized")
	}
	logType, logQuota := model.LogTypeConsume, row.QuotaDelta
	if logQuota < 0 {
		logType, logQuota = model.LogTypeRefund, -logQuota
	}
	log := &model.Log{TaskSettlementID: &row.ID, UserId: e.UserID, CreatedAt: row.AppliedAt, Type: logType, Quota: logQuota, ChannelId: e.ChannelID, TokenId: e.TokenID, ModelName: e.ModelName, Group: e.Group, Content: "Task terminal settlement", Other: common.MapToJsonStr(e.Other)}
	if err := model.LOG_DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(log).Error; err != nil {
		return err
	}
	var written model.Log
	if err := model.LOG_DB.WithContext(ctx).Where("task_settlement_id = ?", row.ID).First(&written).Error; err != nil {
		return err
	}
	if written.Quota != logQuota || written.Type != logType || written.UserId != e.UserID {
		if logQuota != 0 && written.Quota == 0 {
			taskZeroSettlementLogs.Add(1)
		}
		return errors.New("task settlement log does not match committed ledger")
	}
	return nil
}
