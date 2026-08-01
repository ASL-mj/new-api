package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// MonitorGroup is an independently configured set of channels for active health probes.
// It is intentionally separate from NewAPI calling groups and billing configuration.
type MonitorGroup struct {
	Id              int    `json:"id"`
	Name            string `json:"name" gorm:"size:100;not null;uniqueIndex"`
	Key             string `json:"key" gorm:"size:64;not null;uniqueIndex"`
	Description     string `json:"description" gorm:"size:255"`
	PrimaryModel    string `json:"primary_model" gorm:"size:128;not null"`
	ExtraModels     string `json:"extra_models" gorm:"type:text"`
	Enabled         bool   `json:"enabled" gorm:"not null;default:true;index"`
	UserVisible     bool   `json:"user_visible" gorm:"not null;default:true;index"`
	IntervalSeconds int    `json:"interval_seconds" gorm:"not null;default:300"`
	TimeoutSeconds  int    `json:"timeout_seconds" gorm:"not null;default:30"`
	DegradedMs      int    `json:"degraded_ms" gorm:"not null;default:3000"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint"`
	LastCheckedAt   int64  `json:"last_checked_at" gorm:"bigint;index"`
	RunLeaseUntil   int64  `json:"-" gorm:"bigint;not null;default:0;index"`
	RunLeaseToken   string `json:"-" gorm:"size:64;not null;default:''"`
}

type MonitorGroupTarget struct {
	Id             int    `json:"id"`
	MonitorGroupId int    `json:"monitor_group_id" gorm:"not null;uniqueIndex:idx_monitor_group_channel,priority:1;index"`
	ChannelId      int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_monitor_group_channel,priority:2;index"`
	Models         string `json:"models" gorm:"type:text"`
	Enabled        bool   `json:"enabled" gorm:"not null;default:true;index"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
}

func (MonitorGroup) TableName() string {
	return "monitor_groups"
}

func (MonitorGroupTarget) TableName() string {
	return "monitor_group_targets"
}

func GetMonitorGroups(pageInfo *common.PageInfo, search string, status int) ([]*MonitorGroup, int64, error) {
	groups := make([]*MonitorGroup, 0)
	tx := DB.Model(&MonitorGroup{})

	search = strings.TrimSpace(search)
	if search != "" {
		pattern := "%" + search + "%"
		keyColumn := commonKeyCol
		if keyColumn == "" {
			keyColumn = "`key`"
		}
		tx = tx.Where("name LIKE ? OR "+keyColumn+" LIKE ?", pattern, pattern)
	}
	switch status {
	case 1:
		tx = tx.Where("enabled = ?", true)
	case 2:
		tx = tx.Where("enabled = ?", false)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if pageInfo != nil {
		tx = tx.Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx())
	}
	if err := tx.Order("id DESC").Find(&groups).Error; err != nil {
		return nil, 0, err
	}
	return groups, total, nil
}

func GetMonitorGroupById(id int) (*MonitorGroup, error) {
	group := &MonitorGroup{}
	if err := DB.First(group, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return group, nil
}

func CreateMonitorGroup(group *MonitorGroup, channelIds []int) error {
	if group == nil {
		return errors.New("monitor group is nil")
	}
	targets, err := newMonitorGroupTargets(channelIds, 0)
	if err != nil {
		return err
	}

	now := common.GetTimestamp()
	enabled := group.Enabled
	userVisible := group.UserVisible
	group.Id = 0
	group.CreatedAt = now
	group.UpdatedAt = now
	group.LastCheckedAt = 0
	group.RunLeaseUntil = 0
	group.RunLeaseToken = ""
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(group).Error; err != nil {
			return err
		}
		// GORM applies default:true to zero-value bools during Create, so persist an
		// explicit false after the row has an ID.
		if err := tx.Model(&MonitorGroup{}).Where("id = ?", group.Id).Updates(map[string]interface{}{
			"enabled":      enabled,
			"user_visible": userVisible,
		}).Error; err != nil {
			return err
		}
		group.Enabled = enabled
		group.UserVisible = userVisible
		for i := range targets {
			targets[i].MonitorGroupId = group.Id
			targets[i].CreatedAt = now
		}
		return tx.Create(&targets).Error
	})
}

func UpdateMonitorGroup(group *MonitorGroup, channelIds []int) error {
	if group == nil || group.Id <= 0 {
		return errors.New("invalid monitor group")
	}
	targets, err := newMonitorGroupTargets(channelIds, group.Id)
	if err != nil {
		return err
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var existing MonitorGroup
		if err := tx.First(&existing, "id = ?", group.Id).Error; err != nil {
			return err
		}
		if group.Key != "" && group.Key != existing.Key {
			return errors.New("monitor group key cannot be changed")
		}

		group.Key = existing.Key
		group.CreatedAt = existing.CreatedAt
		group.UpdatedAt = common.GetTimestamp()
		group.LastCheckedAt = existing.LastCheckedAt
		group.RunLeaseUntil = existing.RunLeaseUntil
		group.RunLeaseToken = existing.RunLeaseToken
		if err := tx.Save(group).Error; err != nil {
			return err
		}
		if err := tx.Where("monitor_group_id = ?", group.Id).Delete(&MonitorGroupTarget{}).Error; err != nil {
			return err
		}
		for i := range targets {
			targets[i].CreatedAt = group.UpdatedAt
		}
		return tx.Create(&targets).Error
	})
}

func TryAcquireMonitorGroupRunLease(id int, token string, now, leaseUntil int64) (bool, error) {
	if id <= 0 || token == "" || leaseUntil <= now {
		return false, errors.New("invalid monitor group run lease")
	}
	result := DB.Model(&MonitorGroup{}).
		Where("id = ? AND run_lease_until <= ?", id, now).
		Updates(map[string]interface{}{
			"run_lease_until": leaseUntil,
			"run_lease_token": token,
		})
	return result.RowsAffected == 1, result.Error
}

func ReleaseMonitorGroupRunLease(id int, token string) error {
	if id <= 0 || token == "" {
		return nil
	}
	return DB.Model(&MonitorGroup{}).
		Where("id = ? AND run_lease_token = ?", id, token).
		Updates(map[string]interface{}{
			"run_lease_until": int64(0),
			"run_lease_token": "",
		}).Error
}

func DeleteMonitorGroup(id int) error {
	if id <= 0 {
		return errors.New("invalid monitor group id")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("monitor_group_id = ?", id).Delete(&MonitorGroupTarget{}).Error; err != nil {
			return err
		}
		return tx.Delete(&MonitorGroup{}, id).Error
	})
}

func GetEnabledMonitorGroups() ([]*MonitorGroup, error) {
	groups := make([]*MonitorGroup, 0)
	err := DB.Where("enabled = ?", true).Order("id ASC").Find(&groups).Error
	return groups, err
}

func GetMonitorGroupTargets(groupId int) ([]*MonitorGroupTarget, error) {
	targets := make([]*MonitorGroupTarget, 0)
	err := DB.Where("monitor_group_id = ?", groupId).Order("id ASC").Find(&targets).Error
	return targets, err
}

func GetMonitorGroupTargetsByGroupIds(groupIds []int) ([]*MonitorGroupTarget, error) {
	if len(groupIds) == 0 {
		return []*MonitorGroupTarget{}, nil
	}
	targets := make([]*MonitorGroupTarget, 0)
	err := DB.Where("monitor_group_id IN (?)", groupIds).
		Order("monitor_group_id ASC, id ASC").
		Find(&targets).Error
	return targets, err
}

func newMonitorGroupTargets(channelIds []int, groupId int) ([]MonitorGroupTarget, error) {
	if len(channelIds) == 0 {
		return nil, errors.New("monitor group requires at least one channel")
	}

	seen := make(map[int]struct{}, len(channelIds))
	targets := make([]MonitorGroupTarget, 0, len(channelIds))
	for _, channelId := range channelIds {
		if channelId <= 0 {
			return nil, fmt.Errorf("invalid channel id: %d", channelId)
		}
		if _, exists := seen[channelId]; exists {
			continue
		}
		seen[channelId] = struct{}{}
		targets = append(targets, MonitorGroupTarget{
			MonitorGroupId: groupId,
			ChannelId:      channelId,
			Enabled:        true,
		})
	}
	return targets, nil
}
