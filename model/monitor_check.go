package model

import (
	"errors"
	"fmt"
	"time"
)

const (
	MonitorCheckStatusOperational = "operational"
	MonitorCheckStatusDegraded    = "degraded"
	MonitorCheckStatusFailed      = "failed"
	MonitorCheckStatusTimeout     = "timeout"
)

type MonitorCheck struct {
	Id             int    `json:"id"`
	MonitorGroupId int    `json:"monitor_group_id" gorm:"not null;index:idx_monitor_check_group_time,priority:1"`
	ChannelId      int    `json:"channel_id" gorm:"not null;index"`
	ModelName      string `json:"model_name" gorm:"size:128;not null;index"`
	Status         string `json:"status" gorm:"size:16;not null;index"`
	LatencyMs      int64  `json:"latency_ms" gorm:"not null;default:0"`
	PingLatencyMs  *int64 `json:"ping_latency_ms"`
	ErrorCode      string `json:"error_code" gorm:"size:64"`
	ErrorMessage   string `json:"error_message" gorm:"type:text"`
	CheckedAt      int64  `json:"checked_at" gorm:"bigint;not null;index:idx_monitor_check_group_time,priority:2;index"`
}

type MonitorAvailability struct {
	BucketTs        int64   `json:"bucket_ts"`
	CheckCount      int64   `json:"check_count"`
	AvailableCount  int64   `json:"available_count"`
	AvailabilityPct float64 `json:"availability_pct"`
}

func (MonitorCheck) TableName() string {
	return "monitor_checks"
}

func InsertMonitorChecks(rows []*MonitorCheck) error {
	if len(rows) == 0 {
		return nil
	}
	return DB.CreateInBatches(rows, 100).Error
}

func GetLatestMonitorChecks(groupIds []int) ([]*MonitorCheck, error) {
	if groupIds != nil && len(groupIds) == 0 {
		return []*MonitorCheck{}, nil
	}

	latest := DB.Model(&MonitorCheck{}).
		Select("monitor_group_id, channel_id, model_name, MAX(checked_at) AS checked_at")
	if groupIds != nil {
		latest = latest.Where("monitor_group_id IN (?)", groupIds)
	}
	latest = latest.Group("monitor_group_id, channel_id, model_name")

	matchedChecks := make([]*MonitorCheck, 0)
	err := DB.Table("monitor_checks AS checks").
		Select("checks.*").
		Joins("JOIN (?) AS latest ON latest.monitor_group_id = checks.monitor_group_id AND latest.channel_id = checks.channel_id AND latest.model_name = checks.model_name AND latest.checked_at = checks.checked_at", latest).
		Order("checks.monitor_group_id ASC, checks.channel_id ASC, checks.model_name ASC, checks.id DESC").
		Scan(&matchedChecks).Error
	if err != nil {
		return nil, err
	}
	checks := make([]*MonitorCheck, 0, len(matchedChecks))
	seen := make(map[string]struct{}, len(matchedChecks))
	for _, check := range matchedChecks {
		key := fmt.Sprintf("%d\x00%d\x00%s", check.MonitorGroupId, check.ChannelId, check.ModelName)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		checks = append(checks, check)
	}
	return checks, nil
}

func GetMonitorTimeline(groupId int, modelName string, limit int) ([]*MonitorCheck, error) {
	if groupId <= 0 {
		return nil, errors.New("invalid monitor group id")
	}
	if limit <= 0 {
		limit = 60
	}
	if limit > 1000 {
		limit = 1000
	}

	tx := DB.Where("monitor_group_id = ?", groupId)
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	checks := make([]*MonitorCheck, 0)
	if err := tx.Order("checked_at DESC, id DESC").Limit(limit).Find(&checks).Error; err != nil {
		return nil, err
	}
	for left, right := 0, len(checks)-1; left < right; left, right = left+1, right-1 {
		checks[left], checks[right] = checks[right], checks[left]
	}
	return checks, nil
}

func GetMonitorChecksForStatus(groupIds []int, checkedAfter int64) ([]*MonitorCheck, error) {
	if len(groupIds) == 0 {
		return []*MonitorCheck{}, nil
	}
	checks := make([]*MonitorCheck, 0)
	err := DB.Where("monitor_group_id IN (?) AND checked_at >= ?", groupIds, checkedAfter).
		Order("monitor_group_id ASC, checked_at ASC, id ASC").
		Find(&checks).Error
	return checks, err
}

func GetMonitorAvailability(groupId int, days int) ([]MonitorAvailability, error) {
	if groupId <= 0 {
		return nil, errors.New("invalid monitor group id")
	}
	if days <= 0 {
		return []MonitorAvailability{}, nil
	}

	from := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	availability := make([]MonitorAvailability, 0)
	// Modulo-based daily buckets work across SQLite, MySQL, and PostgreSQL.
	err := DB.Model(&MonitorCheck{}).
		Select(`checked_at - (checked_at % 86400) AS bucket_ts,
			COUNT(*) AS check_count,
			SUM(CASE WHEN status IN ('operational', 'degraded') THEN 1 ELSE 0 END) AS available_count`).
		Where("monitor_group_id = ? AND checked_at >= ?", groupId, from).
		Group("checked_at - (checked_at % 86400)").
		Order("bucket_ts ASC").
		Scan(&availability).Error
	if err != nil {
		return nil, err
	}
	for i := range availability {
		if availability[i].CheckCount > 0 {
			availability[i].AvailabilityPct = float64(availability[i].AvailableCount) * 100 / float64(availability[i].CheckCount)
		}
	}
	return availability, nil
}

func DeleteExpiredMonitorChecks(expireBefore int64) (int64, error) {
	result := DB.Where("checked_at < ?", expireBefore).Delete(&MonitorCheck{})
	return result.RowsAffected, result.Error
}
