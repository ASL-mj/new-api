package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// The ledger remains historical; only its detail projection follows the task's
// latest settlement. Match both public ID and owner before attaching facts.
func projectTaskLogBilling(logs []*model.Log, role int) error {
	ids := make([]string, 0, len(logs))
	owners := make([]int, 0, len(logs))
	metadata := make(map[*model.Log]map[string]any)
	for _, log := range logs {
		var other map[string]any
		if common.Unmarshal([]byte(log.Other), &other) != nil || other == nil {
			continue
		}
		delete(other, "task_billing")
		log.Other = common.MapToJsonStr(other)
		id, _ := other["task_id"].(string)
		if !publicVideoTaskID.MatchString(id) {
			continue
		}
		metadata[log] = other
		ids = append(ids, id)
		owners = append(owners, log.UserId)
	}
	if len(ids) == 0 {
		return nil
	}
	var tasks []*model.Task
	if err := model.DB.Where("task_id IN ? AND user_id IN ? AND polling_version = ?", ids, owners, 2).Find(&tasks).Error; err != nil {
		return err
	}
	settlements, err := taskVideoSettlements(tasks)
	if err != nil {
		return err
	}
	type identity struct {
		id    string
		owner int
	}
	byIdentity := make(map[identity]*model.Task, len(tasks))
	for _, task := range tasks {
		byIdentity[identity{task.TaskID, task.UserId}] = task
	}
	for log, other := range metadata {
		id, _ := other["task_id"].(string)
		task := byIdentity[identity{id, log.UserId}]
		if task == nil {
			continue
		}
		item := &dto.TaskDto{}
		projectTaskVideo(item, task, settlements[task.ID], role)
		billing := map[string]any{
			"task_id": task.TaskID, "model": item.Model,
			"pre_consumed_quota": item.PreConsumedQuota, "actual_quota": item.ActualQuota,
			"quota_delta": item.QuotaDelta, "settlement_status": item.SettlementStatus,
			"settlement_kind": item.SettlementKind, "settled_at": item.SettledAt,
		}
		if role >= common.RoleAdminUser {
			billing["poll_failures"] = item.PollFailures
			billing["last_poll_at"] = item.LastPollAt
			billing["next_poll_at"] = item.NextPollAt
			billing["last_poll_error_class"] = item.LastPollErrorClass
			billing["last_poll_error_message"] = item.LastPollErrorMessage
		}
		if role >= common.RoleRootUser {
			billing["settlement_id"] = item.SettlementID
			billing["pricing_version"] = item.PricingVersion
		}
		other["task_billing"] = billing
		log.Other = common.MapToJsonStr(other)
	}
	return nil
}
