package model

import "fmt"

// GetReadyTaskPolls excludes unbound submissions and respects retry/lease times.
func GetReadyTaskPolls(now int64, limit int) ([]*Task, error) {
	var tasks []*Task
	err := DB.Where("status IN ?", []TaskStatus{TaskStatusSubmitted, TaskStatusQueued, TaskStatusInProgress}).
		Where("polling_version = ?", 2).
		Where("submission_settled = ?", true).
		Where("next_poll_at <= ?", now).Order("next_poll_at, id").Limit(limit).Find(&tasks).Error
	return tasks, err
}

// ClaimTaskPoll uses the observed schedule as a cross-database lease CAS.
func ClaimTaskPoll(task *Task, now, leaseUntil int64) (bool, error) {
	if task == nil || task.ID == 0 || leaseUntil <= now {
		return false, fmt.Errorf("invalid task poll lease")
	}
	result := DB.Model(&Task{}).Where("id = ? AND status = ? AND next_poll_at = ? AND last_poll_at = ?", task.ID, task.Status, task.NextPollAt, task.LastPollAt).
		Updates(map[string]any{"last_poll_at": now, "next_poll_at": leaseUntil})
	if result.Error != nil || result.RowsAffected == 0 {
		return false, result.Error
	}
	task.LastPollAt = now
	task.NextPollAt = leaseUntil
	return true, nil
}

// SaveTaskPoll prevents an expired poll from overwriting a newer lease/result.
func SaveTaskPoll(task *Task, from TaskStatus, leaseUntil int64) (bool, error) {
	result := DB.Model(&Task{}).Where("id = ? AND status = ? AND next_poll_at = ? AND last_poll_at = ?", task.ID, from, leaseUntil, task.LastPollAt).
		Select("status", "progress", "start_time", "finish_time", "fail_reason", "data", "private_data", "poll_failures", "last_poll_at", "next_poll_at", "last_poll_error_class", "last_poll_error_message", "updated_at").Updates(task)
	return result.RowsAffected > 0, result.Error
}
