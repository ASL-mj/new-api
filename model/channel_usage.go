package model

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ChannelKeyFingerprintSecretOption = "ChannelKeyFingerprintSecret"

const (
	channelUsageUsedQuotaIncrementSQL      = "used_quota + ?"
	channelUsageQuotaLimitIncrementSQL     = "quota_limit_used + CASE WHEN quota_limit > 0 AND quota_limit_mode IN ? THEN ? ELSE 0 END"
	channelUsageAutoDisableConditionSQL    = "id = ? AND status = ? AND quota_limit > 0 AND quota_limit_mode IN ? AND quota_limit_used >= quota_limit"
	channelKeyUsageQuotaLimitIncrementSQL  = "quota_limit_used + CASE WHEN quota_limit > 0 THEN ? ELSE 0 END"
	channelKeyUsageAutoDisableConditionSQL = "channel_id = ? AND key_fingerprint = ? AND status = ? AND quota_limit > 0 AND quota_limit_used >= quota_limit"
	channelKeyQuotaDisabledReason          = "key quota limit reached"
)

var channelKeyFingerprintSecretState struct {
	sync.RWMutex
	value string
	ready bool
}

type ChannelKeyUsage struct {
	Id                int    `json:"id"`
	ChannelId         int    `json:"channel_id" gorm:"uniqueIndex:idx_channel_key_fingerprint,priority:1;index"`
	KeyFingerprint    string `json:"key_fingerprint" gorm:"size:64;uniqueIndex:idx_channel_key_fingerprint,priority:2"`
	KeyIndex          int    `json:"key_index" gorm:"default:0"`
	KeyMask           string `json:"key_mask" gorm:"size:64"`
	QuotaLimit        int64  `json:"quota_limit" gorm:"bigint;default:0"`
	QuotaLimitUsed    int64  `json:"quota_limit_used" gorm:"bigint;default:0"`
	QuotaLimitResetAt int64  `json:"quota_limit_reset_at" gorm:"bigint;default:0"`
	Status            int    `json:"status" gorm:"default:1;index"`
	DisabledReason    string `json:"disabled_reason" gorm:"size:255"`
	DisabledAt        int64  `json:"disabled_at" gorm:"bigint;default:0"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt         int64  `json:"updated_at" gorm:"bigint"`
}

func (ChannelKeyUsage) TableName() string {
	return "channel_key_usages"
}

func FingerprintChannelKey(key string) (string, error) {
	secret, err := getChannelKeyFingerprintSecret()
	if err != nil {
		return "", err
	}
	return common.GenerateHMACWithKey([]byte(secret), key), nil
}

func MaskChannelKey(key string) string {
	return MaskTokenKey(key)
}

type ChannelUsageDaily struct {
	Id             int    `json:"id"`
	ChannelId      int    `json:"channel_id" gorm:"uniqueIndex:idx_channel_usage_day,priority:1;index"`
	KeyFingerprint string `json:"key_fingerprint" gorm:"size:64;uniqueIndex:idx_channel_usage_day,priority:2"`
	UsageDate      string `json:"usage_date" gorm:"size:10;uniqueIndex:idx_channel_usage_day,priority:3;index"`
	Quota          int64  `json:"quota" gorm:"bigint;default:0"`
	TokenUsed      int64  `json:"token_used" gorm:"bigint;default:0"`
	RequestCount   int64  `json:"request_count" gorm:"bigint;default:0"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

func (ChannelUsageDaily) TableName() string {
	return "channel_usage_daily"
}

type ChannelUsageStats struct {
	ChannelID           int     `json:"channel_id"`
	TodayQuota          int64   `json:"today_quota"`
	TodayTokenUsed      int64   `json:"today_token_used"`
	TodayRequestCount   int64   `json:"today_request_count"`
	Last30dQuota        int64   `json:"last30d_quota"`
	Last30dTokenUsed    int64   `json:"last30d_token_used"`
	Last30dRequestCount int64   `json:"last30d_request_count"`
	QuotaLimitUsed      int64   `json:"quota_limit_used"`
	QuotaLimit          int64   `json:"quota_limit"`
	Balance             float64 `json:"balance"`
}

type channelUsageDailyAggregate struct {
	ChannelID           int
	TodayQuota          int64
	TodayTokenUsed      int64
	TodayRequestCount   int64
	Last30dQuota        int64
	Last30dTokenUsed    int64
	Last30dRequestCount int64
}

type ChannelUsageApplyResult struct {
	UsedQuota            int64
	QuotaLimitUsed       int64
	QuotaLimit           int64
	Status               int
	ChannelJustExhausted bool
}

type ChannelKeyUsageApplyResult struct {
	KeyFingerprint       string
	KeyIndex             int
	QuotaLimitUsed       int64
	QuotaLimit           int64
	Status               int
	ChannelStatus        int
	KeyJustExhausted     bool
	ChannelJustExhausted bool
}

func (usage ChannelKeyUsage) IsQuotaExceeded() bool {
	if usage.QuotaLimit <= 0 {
		return false
	}
	return usage.QuotaLimitUsed >= usage.QuotaLimit
}

func RecordChannelUsageDaily(channelID int, keyFingerprint string, quota int64, tokenUsed int64, requestCount int64, now time.Time) error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	if channelID <= 0 {
		return errors.New("invalid channel id")
	}

	usageDate, err := channelUsageDateFromTime(now)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	updatedAt := now.Unix()

	summaryDelta := ChannelUsageDaily{
		ChannelId:      channelID,
		KeyFingerprint: "",
		UsageDate:      usageDate,
		Quota:          quota,
		TokenUsed:      tokenUsed,
		RequestCount:   requestCount,
		UpdatedAt:      updatedAt,
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := UpsertChannelUsageDaily(tx, summaryDelta); err != nil {
			return err
		}
		if keyFingerprint == "" {
			return nil
		}

		keyDelta := summaryDelta
		keyDelta.KeyFingerprint = keyFingerprint
		_, err := UpsertChannelUsageDaily(tx, keyDelta)
		return err
	})
}

func UpsertChannelUsageDaily(tx *gorm.DB, delta ChannelUsageDaily) (ChannelUsageDaily, error) {
	if tx == nil {
		return ChannelUsageDaily{}, errors.New("transaction is required")
	}
	if delta.ChannelId <= 0 {
		return ChannelUsageDaily{}, errors.New("invalid channel id")
	}
	if delta.UsageDate == "" {
		return ChannelUsageDaily{}, errors.New("usage date is required")
	}
	if delta.UpdatedAt == 0 {
		delta.UpdatedAt = common.GetTimestamp()
	}

	lookup := buildChannelUsageDailyLookup(delta)
	updates := buildChannelUsageDailyUpdates(delta)

	updateResult := tx.Model(&ChannelUsageDaily{}).Where(lookup).Updates(updates)
	if updateResult.Error != nil {
		return ChannelUsageDaily{}, updateResult.Error
	}
	if updateResult.RowsAffected > 0 {
		return readChannelUsageDailyFresh(tx, delta)
	}

	createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&delta)
	if createResult.Error != nil {
		return ChannelUsageDaily{}, createResult.Error
	}
	if createResult.RowsAffected > 0 {
		return readChannelUsageDailyFresh(tx, delta)
	}

	retryUpdate := tx.Model(&ChannelUsageDaily{}).Where(lookup).Updates(updates)
	if retryUpdate.Error != nil {
		return ChannelUsageDaily{}, retryUpdate.Error
	}
	return readChannelUsageDailyFresh(tx, delta)
}

func GetChannelUsageStatsBatch(channelIDs []int, today string, start30d string) (map[int]ChannelUsageStats, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}

	ids := normalizeChannelUsageStatsIDs(channelIDs)
	results := make(map[int]ChannelUsageStats, len(ids))
	for _, channelID := range ids {
		results[channelID] = ChannelUsageStats{ChannelID: channelID}
	}
	if len(ids) == 0 {
		return results, nil
	}

	for _, chunk := range chunkChannelUsageStatsIDs(ids, 200) {
		var channels []Channel
		if err := DB.Model(&Channel{}).
			Select("id", "quota_limit_used", "quota_limit", "balance").
			Where("id IN ?", chunk).
			Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			stat := results[channel.Id]
			stat.QuotaLimitUsed = channel.QuotaLimitUsed
			stat.QuotaLimit = channel.QuotaLimit
			stat.Balance = channel.Balance
			results[channel.Id] = stat
		}

		var aggregates []channelUsageDailyAggregate
		if err := DB.Model(&ChannelUsageDaily{}).
			Select(
				"channel_id, "+
					"SUM(CASE WHEN usage_date = ? THEN quota ELSE 0 END) AS today_quota, "+
					"SUM(CASE WHEN usage_date = ? THEN token_used ELSE 0 END) AS today_token_used, "+
					"SUM(CASE WHEN usage_date = ? THEN request_count ELSE 0 END) AS today_request_count, "+
					"SUM(quota) AS last30d_quota, "+
					"SUM(token_used) AS last30d_token_used, "+
					"SUM(request_count) AS last30d_request_count",
				today,
				today,
				today,
			).
			Where("channel_id IN ? AND key_fingerprint = ? AND usage_date >= ? AND usage_date <= ?", chunk, "", start30d, today).
			Group("channel_id").
			Find(&aggregates).Error; err != nil {
			return nil, err
		}
		for _, aggregate := range aggregates {
			stat := results[aggregate.ChannelID]
			stat.TodayQuota = aggregate.TodayQuota
			stat.TodayTokenUsed = aggregate.TodayTokenUsed
			stat.TodayRequestCount = aggregate.TodayRequestCount
			stat.Last30dQuota = aggregate.Last30dQuota
			stat.Last30dTokenUsed = aggregate.Last30dTokenUsed
			stat.Last30dRequestCount = aggregate.Last30dRequestCount
			results[aggregate.ChannelID] = stat
		}
	}

	return results, nil
}

func ApplyChannelUsage(channelID int, quota int) (ChannelUsageApplyResult, error) {
	var result ChannelUsageApplyResult
	if DB == nil {
		return result, errors.New("database not initialized")
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if quota > 0 {
			updateResult := tx.Model(&Channel{}).
				Where("id = ?", channelID).
				Updates(buildChannelUsageUpdates(quota))
			if updateResult.Error != nil {
				return updateResult.Error
			}
			if updateResult.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}

			disableConditionSQL, disableConditionArgs := buildChannelUsageAutoDisableCondition(channelID)
			statusUpdateResult := tx.Model(&Channel{}).
				Where(disableConditionSQL, disableConditionArgs...).
				Update("status", common.ChannelStatusAutoDisabled)
			if statusUpdateResult.Error != nil {
				return statusUpdateResult.Error
			}
			result.ChannelJustExhausted = statusUpdateResult.RowsAffected == 1
		}

		var channel Channel
		if err := tx.Select("used_quota", "quota_limit_used", "quota_limit", "status").
			First(&channel, channelID).Error; err != nil {
			return err
		}

		result.UsedQuota = channel.UsedQuota
		result.QuotaLimitUsed = channel.QuotaLimitUsed
		result.QuotaLimit = channel.QuotaLimit
		result.Status = channel.Status
		return nil
	})

	return result, err
}

func buildChannelUsageUpdates(quota int) map[string]interface{} {
	quotaLimitExprSQL, quotaLimitExprArgs := buildChannelQuotaLimitIncrementExpr(quota)
	return map[string]interface{}{
		"used_quota":       gorm.Expr(channelUsageUsedQuotaIncrementSQL, quota),
		"quota_limit_used": gorm.Expr(quotaLimitExprSQL, quotaLimitExprArgs...),
	}
}

func buildChannelQuotaLimitIncrementExpr(quota int) (string, []interface{}) {
	return channelUsageQuotaLimitIncrementSQL, []interface{}{channelUsageQuotaLimitModes(), quota}
}

func buildChannelUsageAutoDisableCondition(channelID int) (string, []interface{}) {
	return channelUsageAutoDisableConditionSQL, []interface{}{
		channelID,
		common.ChannelStatusEnabled,
		channelUsageQuotaLimitModes(),
	}
}

func channelUsageQuotaLimitModes() []string {
	return []string{ChannelQuotaLimitModeChannel, ChannelQuotaLimitModeBoth}
}

func buildChannelUsageDailyLookup(delta ChannelUsageDaily) map[string]interface{} {
	return map[string]interface{}{
		"channel_id":      delta.ChannelId,
		"key_fingerprint": delta.KeyFingerprint,
		"usage_date":      delta.UsageDate,
	}
}

func buildChannelUsageDailyUpdates(delta ChannelUsageDaily) map[string]interface{} {
	return map[string]interface{}{
		"quota":         gorm.Expr("quota + ?", delta.Quota),
		"token_used":    gorm.Expr("token_used + ?", delta.TokenUsed),
		"request_count": gorm.Expr("request_count + ?", delta.RequestCount),
		"updated_at":    delta.UpdatedAt,
	}
}

func buildChannelKeyUsageUpdates(quota int) map[string]interface{} {
	quotaLimitExprSQL, quotaLimitExprArgs := buildChannelKeyUsageQuotaLimitIncrementExpr(quota)
	return map[string]interface{}{
		"quota_limit_used": gorm.Expr(quotaLimitExprSQL, quotaLimitExprArgs...),
	}
}

func buildChannelKeyUsageQuotaLimitIncrementExpr(quota int) (string, []interface{}) {
	return channelKeyUsageQuotaLimitIncrementSQL, []interface{}{quota}
}

func buildChannelKeyUsageAutoDisableCondition(channelID int, fingerprint string) (string, []interface{}) {
	return channelKeyUsageAutoDisableConditionSQL, []interface{}{
		channelID,
		fingerprint,
		common.ChannelStatusEnabled,
	}
}

type channelKeyMeta struct {
	Fingerprint string
	Index       int
	Mask        string
}

type multiKeyFingerprintState struct {
	Status int
	Reason string
	Time   int64
}

func buildChannelKeyMetas(keys []string) ([]channelKeyMeta, error) {
	metas := make([]channelKeyMeta, 0, len(keys))
	for idx, key := range keys {
		fingerprint, err := FingerprintChannelKey(key)
		if err != nil {
			return nil, err
		}
		metas = append(metas, channelKeyMeta{
			Fingerprint: fingerprint,
			Index:       idx,
			Mask:        MaskChannelKey(key),
		})
	}
	return metas, nil
}

func buildChannelKeyFingerprintsByIndexFromRawKey(rawKey string) (map[int]string, error) {
	keys := (&Channel{Key: rawKey}).GetKeys()
	metas, err := buildChannelKeyMetas(keys)
	if err != nil {
		return nil, err
	}

	result := make(map[int]string, len(metas))
	for _, meta := range metas {
		result[meta.Index] = meta.Fingerprint
	}
	return result, nil
}

func normalizeChannelInfoStatusMaps(info *ChannelInfo) {
	if info == nil {
		return
	}
	if len(info.MultiKeyStatusList) == 0 {
		info.MultiKeyStatusList = nil
	}
	if len(info.MultiKeyDisabledReason) == 0 {
		info.MultiKeyDisabledReason = nil
	}
	if len(info.MultiKeyDisabledTime) == 0 {
		info.MultiKeyDisabledTime = nil
	}
}

func persistChannelInfo(tx *gorm.DB, channel *Channel) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("database not initialized")
	}
	return tx.Model(channel).Update("channel_info", channel.ChannelInfo).Error
}

func remapChannelInfoByFingerprint(info *ChannelInfo, currentKeys []channelKeyMeta, previousFingerprintsByIndex map[int]string) bool {
	if info == nil {
		return false
	}

	normalizeChannelInfoStatusMaps(info)

	previousStates := make(map[string]multiKeyFingerprintState, len(info.MultiKeyStatusList))
	for index, status := range info.MultiKeyStatusList {
		fingerprint, ok := previousFingerprintsByIndex[index]
		if !ok || fingerprint == "" {
			continue
		}
		previousStates[fingerprint] = multiKeyFingerprintState{
			Status: status,
			Reason: info.MultiKeyDisabledReason[index],
			Time:   info.MultiKeyDisabledTime[index],
		}
	}

	remappedStatus := make(map[int]int)
	remappedReason := make(map[int]string)
	remappedTime := make(map[int]int64)
	for _, currentKey := range currentKeys {
		state, ok := previousStates[currentKey.Fingerprint]
		if !ok || state.Status == 0 || state.Status == common.ChannelStatusEnabled {
			continue
		}
		remappedStatus[currentKey.Index] = state.Status
		if state.Reason != "" {
			remappedReason[currentKey.Index] = state.Reason
		}
		if state.Time != 0 {
			remappedTime[currentKey.Index] = state.Time
		}
	}

	if len(remappedStatus) == 0 {
		remappedStatus = nil
	}
	if len(remappedReason) == 0 {
		remappedReason = nil
	}
	if len(remappedTime) == 0 {
		remappedTime = nil
	}

	changed := info.MultiKeySize != len(currentKeys) ||
		!reflect.DeepEqual(info.MultiKeyStatusList, remappedStatus) ||
		!reflect.DeepEqual(info.MultiKeyDisabledReason, remappedReason) ||
		!reflect.DeepEqual(info.MultiKeyDisabledTime, remappedTime)

	info.MultiKeySize = len(currentKeys)
	info.MultiKeyStatusList = remappedStatus
	info.MultiKeyDisabledReason = remappedReason
	info.MultiKeyDisabledTime = remappedTime
	return changed
}

func ensureChannelKeyUsageRecordsWithPrevious(tx *gorm.DB, channel *Channel, previousFingerprintsByIndex map[int]string) (map[int]*ChannelKeyUsage, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("database not initialized")
	}

	keys := channel.GetKeys()
	currentUsages := make(map[int]*ChannelKeyUsage, len(keys))
	if len(keys) == 0 {
		return currentUsages, nil
	}

	currentKeys, err := buildChannelKeyMetas(keys)
	if err != nil {
		return nil, err
	}

	var existingUsages []ChannelKeyUsage
	if err := tx.Where("channel_id = ?", channel.Id).Find(&existingUsages).Error; err != nil {
		return nil, err
	}

	existingByFingerprint := make(map[string]*ChannelKeyUsage, len(existingUsages))
	for i := range existingUsages {
		existingByFingerprint[existingUsages[i].KeyFingerprint] = &existingUsages[i]
	}

	channelInfoChanged := false
	if len(previousFingerprintsByIndex) > 0 {
		channelInfoChanged = remapChannelInfoByFingerprint(&channel.ChannelInfo, currentKeys, previousFingerprintsByIndex)
	}

	now := common.GetTimestamp()
	createQuotaLimit := int64(0)
	if channel.UsesKeyQuota() && channel.QuotaLimit > 0 {
		createQuotaLimit = channel.QuotaLimit
	}

	needsRefresh := false
	creates := make([]ChannelKeyUsage, 0)
	for _, currentKey := range currentKeys {
		if usage, ok := existingByFingerprint[currentKey.Fingerprint]; ok {
			updates := map[string]interface{}{}
			if usage.KeyIndex != currentKey.Index {
				updates["key_index"] = currentKey.Index
			}
			if usage.KeyMask != currentKey.Mask {
				updates["key_mask"] = currentKey.Mask
			}
			if len(updates) > 0 {
				needsRefresh = true
				updates["updated_at"] = now
				if err := tx.Model(&ChannelKeyUsage{}).
					Where("id = ?", usage.Id).
					Updates(updates).Error; err != nil {
					return nil, err
				}
			}
			continue
		}

		creates = append(creates, ChannelKeyUsage{
			ChannelId:         channel.Id,
			KeyFingerprint:    currentKey.Fingerprint,
			KeyIndex:          currentKey.Index,
			KeyMask:           currentKey.Mask,
			QuotaLimit:        createQuotaLimit,
			QuotaLimitResetAt: channel.QuotaLimitResetAt,
			Status:            common.ChannelStatusEnabled,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
	}

	if len(creates) > 0 {
		needsRefresh = true
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&creates).Error; err != nil {
			return nil, err
		}
	}

	if channelInfoChanged {
		if err := persistChannelInfo(tx, channel); err != nil {
			return nil, err
		}
	}

	var refreshedUsages []ChannelKeyUsage
	if needsRefresh {
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Where("channel_id = ?", channel.Id).
			Find(&refreshedUsages).Error; err != nil {
			return nil, err
		}
	} else {
		refreshedUsages = existingUsages
	}

	refreshedByFingerprint := make(map[string]*ChannelKeyUsage, len(refreshedUsages))
	for i := range refreshedUsages {
		refreshedByFingerprint[refreshedUsages[i].KeyFingerprint] = &refreshedUsages[i]
	}
	for _, currentKey := range currentKeys {
		usage, ok := refreshedByFingerprint[currentKey.Fingerprint]
		if !ok || usage == nil {
			return nil, fmt.Errorf("channel key usage record missing after reconcile: channel_id=%d key_index=%d", channel.Id, currentKey.Index)
		}
		currentUsages[currentKey.Index] = usage
	}

	return currentUsages, nil
}

func ensureChannelKeyUsageRecords(tx *gorm.DB, channel *Channel) (map[int]*ChannelKeyUsage, error) {
	return ensureChannelKeyUsageRecordsWithPrevious(tx, channel, nil)
}

func EnsureChannelKeyUsageRecords(channel *Channel) (map[int]*ChannelKeyUsage, error) {
	return ensureChannelKeyUsageRecords(DB, channel)
}

func ApplyChannelKeyUsage(channel *Channel, selectedKey string, keyIndex int, quota int) (ChannelKeyUsageApplyResult, error) {
	var result ChannelKeyUsageApplyResult
	if channel == nil {
		return result, errors.New("channel is nil")
	}
	if DB == nil {
		return result, errors.New("database not initialized")
	}

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	if err != nil {
		return result, err
	}

	fingerprint, err := FingerprintChannelKey(selectedKey)
	if err != nil {
		return result, err
	}

	usage, ok := currentUsages[keyIndex]
	if !ok || usage == nil || usage.KeyFingerprint != fingerprint {
		ok = false
		for _, currentUsage := range currentUsages {
			if currentUsage != nil && currentUsage.KeyFingerprint == fingerprint {
				usage = currentUsage
				ok = true
				break
			}
		}
	}
	if !ok || usage == nil {
		return result, gorm.ErrRecordNotFound
	}

	result.KeyFingerprint = fingerprint
	result.KeyIndex = usage.KeyIndex

	err = DB.Transaction(func(tx *gorm.DB) error {
		if quota > 0 && channel.UsesKeyQuota() {
			updateResult := tx.Model(&ChannelKeyUsage{}).
				Where("channel_id = ? AND key_fingerprint = ?", channel.Id, fingerprint).
				Updates(buildChannelKeyUsageUpdates(quota))
			if updateResult.Error != nil {
				return updateResult.Error
			}
			if updateResult.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}

			disableConditionSQL, disableConditionArgs := buildChannelKeyUsageAutoDisableCondition(channel.Id, fingerprint)
			statusUpdateResult := tx.Model(&ChannelKeyUsage{}).
				Where(disableConditionSQL, disableConditionArgs...).
				Updates(map[string]interface{}{
					"status":          common.ChannelStatusAutoDisabled,
					"disabled_reason": channelKeyQuotaDisabledReason,
					"disabled_at":     common.GetTimestamp(),
				})
			if statusUpdateResult.Error != nil {
				return statusUpdateResult.Error
			}
			result.KeyJustExhausted = statusUpdateResult.RowsAffected == 1
		}

		var refreshedUsage ChannelKeyUsage
		if err := tx.Select("key_fingerprint", "key_index", "quota_limit_used", "quota_limit", "status").
			Where("channel_id = ? AND key_fingerprint = ?", channel.Id, fingerprint).
			First(&refreshedUsage).Error; err != nil {
			return err
		}

		var refreshedChannel Channel
		if err := tx.Select("status").First(&refreshedChannel, channel.Id).Error; err != nil {
			return err
		}

		result.KeyFingerprint = refreshedUsage.KeyFingerprint
		result.KeyIndex = refreshedUsage.KeyIndex
		result.QuotaLimitUsed = refreshedUsage.QuotaLimitUsed
		result.QuotaLimit = refreshedUsage.QuotaLimit
		result.Status = refreshedUsage.Status
		result.ChannelStatus = refreshedChannel.Status
		return nil
	})
	if err != nil {
		return result, err
	}

	if !result.KeyJustExhausted {
		return result, nil
	}

	UpdateChannelStatus(channel.Id, selectedKey, common.ChannelStatusAutoDisabled, channelKeyQuotaDisabledReason)

	var refreshedChannel Channel
	if err := DB.Select("status").First(&refreshedChannel, channel.Id).Error; err != nil {
		return result, err
	}
	result.ChannelStatus = refreshedChannel.Status
	result.ChannelJustExhausted = refreshedChannel.Status == common.ChannelStatusAutoDisabled
	return result, nil
}

func applyChannelUsageAndPropagate(channelID int, quota int) error {
	result, err := ApplyChannelUsage(channelID, quota)
	if err != nil {
		return err
	}
	if !result.ChannelJustExhausted {
		return nil
	}
	if err := UpdateAbilityStatus(channelID, false); err != nil {
		common.SysLog(fmt.Sprintf("failed to disable abilities for exhausted channel: channel_id=%d, error=%v", channelID, err))
	}
	CacheUpdateChannelStatus(channelID, common.ChannelStatusAutoDisabled)
	return nil
}

func shouldBatchChannelUsedQuotaUpdate(channelID int) (bool, error) {
	if DB == nil {
		return false, errors.New("database not initialized")
	}

	var channel Channel
	if err := DB.Select("quota_limit_mode", "quota_limit").First(&channel, channelID).Error; err != nil {
		return false, err
	}

	return !(channel.UsesChannelQuota() && channel.QuotaLimit > 0), nil
}

func readChannelUsageDailyFresh(tx *gorm.DB, delta ChannelUsageDaily) (ChannelUsageDaily, error) {
	var row ChannelUsageDaily
	err := tx.Session(&gorm.Session{NewDB: true}).
		Where(buildChannelUsageDailyLookup(delta)).
		First(&row).Error
	return row, err
}

func normalizeChannelUsageStatsIDs(channelIDs []int) []int {
	if len(channelIDs) == 0 {
		return nil
	}

	seen := make(map[int]struct{}, len(channelIDs))
	ids := make([]int, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID <= 0 {
			continue
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		seen[channelID] = struct{}{}
		ids = append(ids, channelID)
	}
	return ids
}

func chunkChannelUsageStatsIDs(channelIDs []int, chunkSize int) [][]int {
	if len(channelIDs) == 0 {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = len(channelIDs)
	}

	chunks := make([][]int, 0, (len(channelIDs)+chunkSize-1)/chunkSize)
	for start := 0; start < len(channelIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(channelIDs) {
			end = len(channelIDs)
		}
		chunks = append(chunks, channelIDs[start:end])
	}
	return chunks
}

func getChannelKeyFingerprintSecret() (string, error) {
	if cryptoSecretProvidedByEnv() {
		return common.CryptoSecret, nil
	}

	channelKeyFingerprintSecretState.RLock()
	if channelKeyFingerprintSecretState.ready {
		value := channelKeyFingerprintSecretState.value
		channelKeyFingerprintSecretState.RUnlock()
		return value, nil
	}
	channelKeyFingerprintSecretState.RUnlock()

	channelKeyFingerprintSecretState.Lock()
	defer channelKeyFingerprintSecretState.Unlock()

	if channelKeyFingerprintSecretState.ready {
		return channelKeyFingerprintSecretState.value, nil
	}

	secret, err := loadOrCreateChannelKeyFingerprintSecret()
	if err != nil {
		return "", err
	}

	channelKeyFingerprintSecretState.value = secret
	channelKeyFingerprintSecretState.ready = true
	return secret, nil
}

func loadOrCreateChannelKeyFingerprintSecret() (string, error) {
	if DB == nil {
		return "", errors.New("channel key fingerprint secret requires initialized database or explicit CRYPTO_SECRET")
	}

	generatedSecret, err := common.GenerateKey()
	if err != nil {
		return "", err
	}

	option, err := readChannelKeyFingerprintSecretFresh(DB)
	switch {
	case err == nil && strings.TrimSpace(option.Value) != "":
		cacheChannelKeyFingerprintSecretOption(option.Value)
		return option.Value, nil
	case err == nil:
		if err := fillEmptyChannelKeyFingerprintSecret(DB, generatedSecret); err != nil {
			return "", err
		}
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return "", err
	default:
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{
			Key:   ChannelKeyFingerprintSecretOption,
			Value: generatedSecret,
		}).Error; err != nil {
			return "", err
		}
	}

	// Always resolve via a fresh session after conflict-recovery paths so we read the
	// committed row that won the primary-key race instead of depending on any in-flight snapshot.
	resolvedOption, err := readChannelKeyFingerprintSecretFresh(DB)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resolvedOption.Value) == "" {
		return "", fmt.Errorf("%s option is empty", ChannelKeyFingerprintSecretOption)
	}

	cacheChannelKeyFingerprintSecretOption(resolvedOption.Value)
	return resolvedOption.Value, nil
}

func fillEmptyChannelKeyFingerprintSecret(db *gorm.DB, generatedSecret string) error {
	result := db.Model(&Option{}).
		Where("key = ? AND value = ?", ChannelKeyFingerprintSecretOption, "").
		Update("value", generatedSecret)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func readChannelKeyFingerprintSecretFresh(db *gorm.DB) (Option, error) {
	var option Option
	err := db.Session(&gorm.Session{NewDB: true}).
		Where("key = ?", ChannelKeyFingerprintSecretOption).
		First(&option).Error
	return option, err
}

func cacheChannelKeyFingerprintSecretOption(secret string) {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[ChannelKeyFingerprintSecretOption] = secret
}

func cryptoSecretProvidedByEnv() bool {
	value, exists := os.LookupEnv("CRYPTO_SECRET")
	return exists && strings.TrimSpace(value) != ""
}

func resetChannelKeyFingerprintSecretCache() {
	channelKeyFingerprintSecretState.Lock()
	defer channelKeyFingerprintSecretState.Unlock()
	channelKeyFingerprintSecretState.value = ""
	channelKeyFingerprintSecretState.ready = false
}
