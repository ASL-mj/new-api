package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// Query only the current page's V2 tasks, never once per item. Settlement rows
// are authoritative even when the task snapshot predates a billing commit.
func taskVideoSettlements(tasks []*model.Task) (map[int64]*model.TaskSettlement, error) {
	result := make(map[int64]*model.TaskSettlement)
	ids := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		if task.PollingVersion == 2 {
			ids = append(ids, task.ID)
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	var rows []*model.TaskSettlement
	err := model.DB.Select("id", "task_id", "kind", "state", "pre_consumed_quota", "actual_quota", "quota_delta", "applied_at").Where("task_id IN ?", ids).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.TaskID] = row
	}
	return result, nil
}

func projectTaskVideo(item *dto.TaskDto, task *model.Task, settlement *model.TaskSettlement, role int) {
	item.Model = task.Properties.OriginModelName
	if role < common.RoleAdminUser {
		item.ChannelId = 0
	}
	if task.PollingVersion != 2 {
		item.SettlementStatus = "legacy"
		return
	}
	item.Data = nil
	item.Properties = model.Properties{OriginModelName: item.Model}
	item.ResultURL = task.PrivateData.ResultURL
	item.FailReason = ""
	if task.Status == model.TaskStatusFailure {
		_, item.FailReason = taskVideoDiagnostic(task.LastPollErrorClass)
		if item.FailReason == "" {
			item.FailReason = "Video task failed"
		}
	}
	item.SettlementStatus = "pending"
	if task.SubmissionSettled {
		item.PreConsumedQuota = common.GetPointer(task.Quota)
	}
	if role >= common.RoleAdminUser {
		item.PollFailures = common.GetPointer(task.PollFailures)
		item.LastPollAt = common.GetPointer(task.LastPollAt)
		item.NextPollAt = common.GetPointer(task.NextPollAt)
		item.LastPollErrorClass, item.LastPollErrorMessage = taskVideoDiagnostic(task.LastPollErrorClass)
	}
	if role >= common.RoleRootUser {
		if billing := task.PrivateData.BillingContext; billing != nil && billing.TaskPricingSnapshot != nil {
			item.PricingVersion = billing.TaskPricingSnapshot.Version
		}
	}
	if settlement == nil {
		return
	}
	item.PreConsumedQuota = common.GetPointer(settlement.PreConsumedQuota)
	item.SettlementStatus = string(settlement.State)
	item.SettlementKind = string(settlement.Kind)
	if settlement.State == model.TaskSettlementStateApplied {
		item.ActualQuota = settlement.ActualQuota
		if settlement.Kind == model.TaskSettlementKindRefund {
			item.ActualQuota = common.GetPointer(0)
		}
		item.QuotaDelta = common.GetPointer(settlement.QuotaDelta)
		item.SettledAt = settlement.AppliedAt
	}
	if role >= common.RoleRootUser {
		item.SettlementID = common.GetPointer(settlement.ID)
	}
}

// Re-project diagnostics from a controlled vocabulary rather than trusting
// stored provider text to be free of credentials or upstream identifiers.
func taskVideoDiagnostic(class string) (string, string) {
	switch class {
	case "":
		return "", ""
	case "transport":
		return class, "Upstream connection interrupted"
	case "rate_limited":
		return class, "Upstream rate limit reached"
	case "server_transient":
		return class, "Upstream temporarily unavailable"
	case "malformed_response":
		return class, "Upstream task response is invalid"
	case "provider_terminal":
		return class, "Upstream task failed or was cancelled"
	case "poll_exhausted":
		return class, "Task polling retry limit reached"
	default:
		return "internal", "Task requires internal configuration or storage repair"
	}
}
