package model

import "strings"

type SystemEventLog struct {
	Id         int    `json:"id"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;not null;index:idx_system_event_time"`
	Level      string `json:"level" gorm:"size:16;not null;index:idx_system_event_level_time,priority:1"`
	Component  string `json:"component" gorm:"size:64;not null;index:idx_system_event_component_time,priority:1"`
	Message    string `json:"message" gorm:"type:text;not null"`
	MessageKey string `json:"message_key,omitempty" gorm:"size:128;index"`
	RequestId  string `json:"request_id" gorm:"size:64;index"`
	ChannelId  int    `json:"channel_id" gorm:"index"`
	ModelName  string `json:"model_name" gorm:"size:128;index"`
	Group      string `json:"group" gorm:"column:group;size:64;index"`
	StatusCode int    `json:"status_code" gorm:"index"`
	LatencyMs  int64  `json:"latency_ms"`
	Extra      string `json:"extra" gorm:"type:text"`
}

func (SystemEventLog) TableName() string { return "system_event_logs" }

type SystemEventLogQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	Level          string
	Component      string
	RequestId      string
}

func InsertSystemEventLogs(rows []SystemEventLog) error {
	if len(rows) == 0 {
		return nil
	}
	return DB.CreateInBatches(rows, 100).Error
}

func GetSystemEventLogs(query SystemEventLogQuery, page, pageSize int) ([]SystemEventLog, int64, error) {
	tx := DB.Model(&SystemEventLog{}).Order("created_at DESC, id DESC")
	if query.StartTimestamp > 0 {
		tx = tx.Where("created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		tx = tx.Where("created_at <= ?", query.EndTimestamp)
	}
	if strings.TrimSpace(query.Level) != "" {
		tx = tx.Where("level = ?", strings.TrimSpace(query.Level))
	}
	if strings.TrimSpace(query.Component) != "" {
		tx = tx.Where("component = ?", strings.TrimSpace(query.Component))
	}
	if strings.TrimSpace(query.RequestId) != "" {
		tx = tx.Where("request_id = ?", strings.TrimSpace(query.RequestId))
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	rows := make([]SystemEventLog, 0)
	err := tx.Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, total, err
}

func DeleteExpiredSystemEventLogs(expireBefore int64) (int64, error) {
	result := DB.Where("created_at < ?", expireBefore).Delete(&SystemEventLog{})
	return result.RowsAffected, result.Error
}
