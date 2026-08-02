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
	ChannelQuotaDisabledReason             = "channel quota limit reached"
	ChannelKeyQuotaDisabledReason          = "key quota limit reached"
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

func GetChannelKeyUsages(channel *Channel) ([]ChannelKeyUsage, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	if !channel.ChannelInfo.IsMultiKey {
		return nil, errors.New("channel is not multi-key")
	}
	current, err := EnsureChannelKeyUsageRecords(channel)
	if err != nil {
		return nil, err
	}
	usages := make([]ChannelKeyUsage, 0, len(current))
	for index := 0; index < len(channel.GetKeys()); index++ {
		if usage := current[index]; usage != nil {
			usages = append(usages, *usage)
		}
	}
	return usages, nil
}

func ResetChannelKeyQuotaUsage(channelID int, fingerprint string, resetAt int64) error {
	if channelID <= 0 || strings.TrimSpace(fingerprint) == "" {
		return errors.New("invalid channel key identity")
	}
	result := DB.Model(&ChannelKeyUsage{}).
		Where("channel_id = ? AND key_fingerprint = ?", channelID, fingerprint).
		Updates(map[string]interface{}{
			"quota_limit_used":     0,
			"quota_limit_reset_at": resetAt,
			"updated_at":           resetAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func UpdateChannelKeyQuotaLimit(channelID int, fingerprint string, quotaLimit int64) error {
	if channelID <= 0 || strings.TrimSpace(fingerprint) == "" {
		return errors.New("invalid channel key identity")
	}
	if quotaLimit < 0 {
		return errors.New("quota limit cannot be negative")
	}
	result := DB.Model(&ChannelKeyUsage{}).
		Where("channel_id = ? AND key_fingerprint = ?", channelID, fingerprint).
		Updates(map[string]interface{}{
			"quota_limit": quotaLimit,
			"updated_at":  common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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

type ChannelUsageSettlementParams struct {
	ChannelID      int
	SelectedKey    string
	KeyIndex       int
	HasKeyIdentity bool
	KeyFingerprint string
	Quota          int
	TokenUsed      int64
	RequestCount   int64
	Now            time.Time
}

type ChannelUsageSettlementResult struct {
	Channel ChannelUsageApplyResult
	Key     *ChannelKeyUsageApplyResult
}

type ChannelUsageDeltaParams struct {
	ChannelID      int
	KeyFingerprint string
	KeyIndex       int
	HasKeyIdentity bool
	QuotaDelta     int
	TokenUsedDelta int64
	RequestDelta   int64
	Now            time.Time
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
	if now.IsZero() {
		now = time.Now()
	}

	return runChannelUsageTransaction(func(tx *gorm.DB) error {
		return recordChannelUsageDailyTx(tx, channelID, keyFingerprint, quota, tokenUsed, requestCount, now)
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
			Select("id", "quota_limit_mode", "quota_limit_used", "quota_limit", "balance").
			Where("id IN ?", chunk).
			Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			stat := results[channel.Id]
			if NormalizeQuotaLimitMode(channel.QuotaLimitMode) != ChannelQuotaLimitModeKey {
				stat.QuotaLimitUsed = channel.QuotaLimitUsed
				stat.QuotaLimit = channel.QuotaLimit
			}
			stat.Balance = channel.Balance
			results[channel.Id] = stat
		}

		var aggregates []channelUsageDailyAggregate
		if err := buildChannelUsageStatsAggregateQuery(DB, chunk, today, start30d).Find(&aggregates).Error; err != nil {
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

	err := runChannelUsageTransaction(func(tx *gorm.DB) error {
		var err error
		result, err = applyChannelUsageTx(tx, channelID, quota)
		return err
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
	if DB == nil {
		return nil, errors.New("database not initialized")
	}
	if channel != nil && len(channel.GetKeys()) > 0 {
		if _, err := getChannelKeyFingerprintSecret(); err != nil {
			return nil, err
		}
	}
	var currentUsages map[int]*ChannelKeyUsage
	err := runChannelUsageTransaction(func(tx *gorm.DB) error {
		var err error
		currentUsages, err = ensureChannelKeyUsageRecords(tx, channel)
		return err
	})
	return currentUsages, err
}

func SetChannelKeyStatuses(channel *Channel, keyIndexes []int, status int, reason string) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	if DB == nil {
		return errors.New("database not initialized")
	}
	if len(keyIndexes) == 0 {
		return nil
	}
	if len(channel.GetKeys()) > 0 {
		if _, err := getChannelKeyFingerprintSecret(); err != nil {
			return err
		}
	}

	var updatedChannel Channel
	parentStatusChanged := false
	err := runChannelUsageTransaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&updatedChannel, channel.Id).Error; err != nil {
			return err
		}
		if !updatedChannel.ChannelInfo.IsMultiKey {
			return errors.New("channel is not multi-key")
		}

		keys := updatedChannel.GetKeys()
		currentUsages, err := ensureChannelKeyUsageRecords(tx, &updatedChannel)
		if err != nil {
			return err
		}

		uniqueIndexes := make(map[int]struct{}, len(keyIndexes))
		for _, keyIndex := range keyIndexes {
			if keyIndex < 0 || keyIndex >= len(keys) {
				return fmt.Errorf("channel key index out of range: %d", keyIndex)
			}
			uniqueIndexes[keyIndex] = struct{}{}
		}

		if status == common.ChannelStatusEnabled && updatedChannel.UsesKeyQuota() {
			for keyIndex := range uniqueIndexes {
				usage := currentUsages[keyIndex]
				if usage != nil && usage.IsQuotaExceeded() {
					return fmt.Errorf("%w: key index %d", ErrChannelKeyQuotaResetRequired, keyIndex)
				}
			}
		}

		beforeStatus := updatedChannel.Status
		now := common.GetTimestamp()
		for keyIndex := range uniqueIndexes {
			usage := currentUsages[keyIndex]
			if usage == nil {
				return fmt.Errorf("channel key usage not found: index=%d", keyIndex)
			}

			usageUpdates := map[string]interface{}{
				"status":     status,
				"updated_at": now,
			}
			if status == common.ChannelStatusEnabled {
				usageUpdates["disabled_reason"] = ""
				usageUpdates["disabled_at"] = 0
				delete(updatedChannel.ChannelInfo.MultiKeyStatusList, keyIndex)
				delete(updatedChannel.ChannelInfo.MultiKeyDisabledReason, keyIndex)
				delete(updatedChannel.ChannelInfo.MultiKeyDisabledTime, keyIndex)
			} else {
				usageUpdates["disabled_reason"] = reason
				usageUpdates["disabled_at"] = now
				if updatedChannel.ChannelInfo.MultiKeyStatusList == nil {
					updatedChannel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
				}
				if updatedChannel.ChannelInfo.MultiKeyDisabledReason == nil {
					updatedChannel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
				}
				if updatedChannel.ChannelInfo.MultiKeyDisabledTime == nil {
					updatedChannel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
				}
				updatedChannel.ChannelInfo.MultiKeyStatusList[keyIndex] = status
				updatedChannel.ChannelInfo.MultiKeyDisabledReason[keyIndex] = reason
				updatedChannel.ChannelInfo.MultiKeyDisabledTime[keyIndex] = now
			}

			if err := tx.Model(&ChannelKeyUsage{}).Where("id = ?", usage.Id).Updates(usageUpdates).Error; err != nil {
				return err
			}
		}

		normalizeChannelInfoStatusMaps(&updatedChannel.ChannelInfo)
		if len(updatedChannel.ChannelInfo.MultiKeyStatusList) >= updatedChannel.ChannelInfo.MultiKeySize {
			updatedChannel.Status = common.ChannelStatusAutoDisabled
			info := updatedChannel.GetOtherInfo()
			info["status_reason"] = "All keys are disabled"
			info["status_time"] = now
			updatedChannel.SetOtherInfo(info)
		} else if status == common.ChannelStatusEnabled &&
			updatedChannel.Status == common.ChannelStatusAutoDisabled &&
			!updatedChannel.IsChannelQuotaExceeded() {
			info := updatedChannel.GetOtherInfo()
			if info["status_reason"] == "All keys are disabled" {
				updatedChannel.Status = common.ChannelStatusEnabled
				delete(info, "status_reason")
				delete(info, "status_time")
				updatedChannel.SetOtherInfo(info)
			}
		}
		parentStatusChanged = beforeStatus != updatedChannel.Status

		return tx.Model(&Channel{}).
			Where("id = ?", updatedChannel.Id).
			Select("channel_info", "status", "other_info").
			Updates(map[string]interface{}{
				"channel_info": updatedChannel.ChannelInfo,
				"status":       updatedChannel.Status,
				"other_info":   updatedChannel.OtherInfo,
			}).Error
	})
	if err != nil {
		return err
	}

	*channel = updatedChannel
	if parentStatusChanged {
		if err := UpdateAbilityStatus(channel.Id, channel.Status == common.ChannelStatusEnabled); err != nil {
			return err
		}
		CacheUpdateChannelStatus(channel.Id, channel.Status)
	}
	return nil
}

func SetChannelKeyStatus(channel *Channel, keyIndex int, status int, reason string) error {
	return SetChannelKeyStatuses(channel, []int{keyIndex}, status, reason)
}

func ApplyChannelKeyUsage(channel *Channel, selectedKey string, keyIndex int, quota int) (ChannelKeyUsageApplyResult, error) {
	var result ChannelKeyUsageApplyResult
	if channel == nil {
		return result, errors.New("channel is nil")
	}
	if DB == nil {
		return result, errors.New("database not initialized")
	}

	err := runChannelUsageTransaction(func(tx *gorm.DB) error {
		var err error
		result, err = applyChannelKeyUsageTx(tx, channel, selectedKey, keyIndex, quota)
		return err
	})
	if err != nil {
		return result, err
	}

	if !result.KeyJustExhausted {
		return result, nil
	}

	UpdateChannelStatus(channel.Id, selectedKey, common.ChannelStatusAutoDisabled, ChannelKeyQuotaDisabledReason)
	if result.ChannelJustExhausted {
		if err := UpdateAbilityStatus(channel.Id, false); err != nil {
			common.SysLog(fmt.Sprintf("failed to disable abilities for exhausted channel: channel_id=%d, error=%v", channel.Id, err))
		}
		CacheUpdateChannelStatus(channel.Id, common.ChannelStatusAutoDisabled)
	}
	return result, nil
}

func ApplyChannelUsageSettlement(params ChannelUsageSettlementParams) (ChannelUsageSettlementResult, error) {
	var result ChannelUsageSettlementResult
	if DB == nil {
		return result, errors.New("database not initialized")
	}
	if params.ChannelID <= 0 {
		return result, errors.New("invalid channel id")
	}
	if params.Quota < 0 {
		params.Quota = 0
	}
	if params.TokenUsed < 0 {
		params.TokenUsed = 0
	}
	if params.RequestCount < 0 {
		params.RequestCount = 0
	}
	if params.Now.IsZero() {
		params.Now = time.Now()
	}

	recordDaily := params.Quota > 0 || params.TokenUsed > 0 || params.RequestCount > 0
	recordKeyUsage := params.HasKeyIdentity && (strings.TrimSpace(params.SelectedKey) != "" || strings.TrimSpace(params.KeyFingerprint) != "")
	if recordKeyUsage {
		if strings.TrimSpace(params.KeyFingerprint) == "" {
			fingerprint, err := FingerprintChannelKey(params.SelectedKey)
			if err != nil {
				return result, err
			}
			params.KeyFingerprint = fingerprint
		}
	}

	err := runChannelUsageTransaction(func(tx *gorm.DB) error {
		keyFingerprint := strings.TrimSpace(params.KeyFingerprint)
		if params.Quota > 0 {
			channelResult, err := applyChannelUsageTx(tx, params.ChannelID, params.Quota)
			if err != nil {
				return err
			}
			result.Channel = channelResult
		}

		if recordKeyUsage && params.Quota > 0 {
			var channel Channel
			if err := tx.First(&channel, params.ChannelID).Error; err != nil {
				return err
			}
			keyResult, err := applyChannelKeyUsageByFingerprintTx(tx, &channel, keyFingerprint, params.KeyIndex, params.Quota)
			if err != nil {
				return err
			}
			result.Key = &keyResult
			keyFingerprint = keyResult.KeyFingerprint
		}

		if !recordDaily {
			return nil
		}

		return recordChannelUsageDailyTx(tx, params.ChannelID, keyFingerprint, int64(params.Quota), params.TokenUsed, params.RequestCount, params.Now)
	})
	return result, err
}

func ApplyChannelUsageDelta(params ChannelUsageDeltaParams) (ChannelUsageSettlementResult, error) {
	var result ChannelUsageSettlementResult
	if DB == nil {
		return result, errors.New("database not initialized")
	}
	if params.ChannelID <= 0 {
		return result, errors.New("invalid channel id")
	}
	if params.Now.IsZero() {
		params.Now = time.Now()
	}
	if params.QuotaDelta == 0 && params.TokenUsedDelta == 0 && params.RequestDelta == 0 {
		return result, nil
	}
	params.KeyFingerprint = strings.TrimSpace(params.KeyFingerprint)
	params.HasKeyIdentity = params.HasKeyIdentity && params.KeyFingerprint != ""

	var updatedChannel Channel
	var parentStatusChanged bool
	var channelInfoChanged bool

	err := runChannelUsageTransaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&updatedChannel, params.ChannelID).Error; err != nil {
			return err
		}
		beforeStatus := updatedChannel.Status

		if params.QuotaDelta != 0 {
			if err := adjustChannelQuotaUsageTx(tx, &updatedChannel, params.QuotaDelta); err != nil {
				return err
			}
		}

		var keyUsage *ChannelKeyUsage
		if params.HasKeyIdentity {
			usage, err := findChannelKeyUsageByFingerprintTx(tx, &updatedChannel, params.KeyFingerprint, params.KeyIndex)
			if err != nil {
				return err
			}
			keyUsage = usage
			if params.QuotaDelta != 0 {
				if err := adjustChannelKeyQuotaUsageTx(tx, &updatedChannel, keyUsage, params.QuotaDelta); err != nil {
					return err
				}
				refreshed, err := findChannelKeyUsageByFingerprintTx(tx, &updatedChannel, params.KeyFingerprint, keyUsage.KeyIndex)
				if err != nil {
					return err
				}
				keyUsage = refreshed
				if err := syncKeyQuotaStatusAfterDeltaTx(tx, &updatedChannel, keyUsage, params.QuotaDelta, params.Now); err != nil {
					return err
				}
				if params.QuotaDelta > 0 && keyUsage.Status == common.ChannelStatusAutoDisabled {
					if _, err := disableChannelWhenNoUsableKeysTx(tx, updatedChannel.Id); err != nil {
						return err
					}
				}
			}
			result.Key = &ChannelKeyUsageApplyResult{
				KeyFingerprint: keyUsage.KeyFingerprint,
				KeyIndex:       keyUsage.KeyIndex,
				QuotaLimitUsed: keyUsage.QuotaLimitUsed,
				QuotaLimit:     keyUsage.QuotaLimit,
				Status:         keyUsage.Status,
			}
		}

		if err := tx.First(&updatedChannel, updatedChannel.Id).Error; err != nil {
			return err
		}
		if err := syncChannelQuotaStatusAfterDeltaTx(tx, &updatedChannel, params.QuotaDelta, params.Now); err != nil {
			return err
		}

		if params.QuotaDelta != 0 || params.TokenUsedDelta != 0 || params.RequestDelta != 0 {
			keyFingerprint := ""
			if params.HasKeyIdentity {
				keyFingerprint = params.KeyFingerprint
			}
			if err := recordSignedChannelUsageDailyTx(tx, params.ChannelID, keyFingerprint, int64(params.QuotaDelta), params.TokenUsedDelta, params.RequestDelta, params.Now); err != nil {
				return err
			}
		}

		if err := tx.First(&updatedChannel, updatedChannel.Id).Error; err != nil {
			return err
		}
		parentStatusChanged = beforeStatus != updatedChannel.Status
		channelInfoChanged = updatedChannel.ChannelInfo.IsMultiKey
		result.Channel = ChannelUsageApplyResult{
			UsedQuota:      updatedChannel.UsedQuota,
			QuotaLimitUsed: updatedChannel.QuotaLimitUsed,
			QuotaLimit:     updatedChannel.QuotaLimit,
			Status:         updatedChannel.Status,
		}
		if result.Key != nil {
			result.Key.ChannelStatus = updatedChannel.Status
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	if parentStatusChanged {
		if err := UpdateAbilityStatus(updatedChannel.Id, updatedChannel.Status == common.ChannelStatusEnabled); err != nil {
			common.SysLog(fmt.Sprintf("failed to update ability status after channel usage delta: channel_id=%d, error=%v", updatedChannel.Id, err))
		}
		CacheUpdateChannelStatus(updatedChannel.Id, updatedChannel.Status)
	}
	if channelInfoChanged {
		CacheUpdateChannel(&updatedChannel)
	}
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

func applyChannelUsageTx(tx *gorm.DB, channelID int, quota int) (ChannelUsageApplyResult, error) {
	var result ChannelUsageApplyResult
	if tx == nil {
		return result, errors.New("transaction is required")
	}

	if quota > 0 {
		updateResult := tx.Model(&Channel{}).
			Where("id = ?", channelID).
			Updates(buildChannelUsageUpdates(quota))
		if updateResult.Error != nil {
			return result, updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return result, gorm.ErrRecordNotFound
		}

		disableConditionSQL, disableConditionArgs := buildChannelUsageAutoDisableCondition(channelID)
		statusUpdateResult := tx.Model(&Channel{}).
			Where(disableConditionSQL, disableConditionArgs...).
			Update("status", common.ChannelStatusAutoDisabled)
		if statusUpdateResult.Error != nil {
			return result, statusUpdateResult.Error
		}
		result.ChannelJustExhausted = statusUpdateResult.RowsAffected == 1
		if result.ChannelJustExhausted {
			var disabledChannel Channel
			if err := tx.Select("id", "other_info").First(&disabledChannel, channelID).Error; err != nil {
				return result, err
			}
			info := disabledChannel.GetOtherInfo()
			info["status_reason"] = ChannelQuotaDisabledReason
			info["status_time"] = common.GetTimestamp()
			disabledChannel.SetOtherInfo(info)
			if err := tx.Model(&Channel{}).Where("id = ?", channelID).Update("other_info", disabledChannel.OtherInfo).Error; err != nil {
				return result, err
			}
		}
	}

	var channel Channel
	if err := tx.Select("used_quota", "quota_limit_used", "quota_limit", "status").
		First(&channel, channelID).Error; err != nil {
		return result, err
	}

	result.UsedQuota = channel.UsedQuota
	result.QuotaLimitUsed = channel.QuotaLimitUsed
	result.QuotaLimit = channel.QuotaLimit
	result.Status = channel.Status
	return result, nil
}

func applyChannelKeyUsageTx(tx *gorm.DB, channel *Channel, selectedKey string, keyIndex int, quota int) (ChannelKeyUsageApplyResult, error) {
	fingerprint, err := FingerprintChannelKey(selectedKey)
	if err != nil {
		return ChannelKeyUsageApplyResult{}, err
	}
	return applyChannelKeyUsageByFingerprintTx(tx, channel, fingerprint, keyIndex, quota)
}

func applyChannelKeyUsageByFingerprintTx(tx *gorm.DB, channel *Channel, fingerprint string, keyIndex int, quota int) (ChannelKeyUsageApplyResult, error) {
	var result ChannelKeyUsageApplyResult
	if tx == nil {
		return result, errors.New("transaction is required")
	}
	if channel == nil {
		return result, errors.New("channel is nil")
	}

	var lockedChannel Channel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedChannel, channel.Id).Error; err != nil {
		return result, err
	}
	channel = &lockedChannel

	currentUsages, err := ensureChannelKeyUsageRecords(tx, channel)
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

	if quota > 0 && channel.UsesKeyQuota() {
		updateResult := tx.Model(&ChannelKeyUsage{}).
			Where("channel_id = ? AND key_fingerprint = ?", channel.Id, fingerprint).
			Updates(buildChannelKeyUsageUpdates(quota))
		if updateResult.Error != nil {
			return result, updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return result, gorm.ErrRecordNotFound
		}

		disableConditionSQL, disableConditionArgs := buildChannelKeyUsageAutoDisableCondition(channel.Id, fingerprint)
		statusUpdateResult := tx.Model(&ChannelKeyUsage{}).
			Where(disableConditionSQL, disableConditionArgs...).
			Updates(map[string]interface{}{
				"status":          common.ChannelStatusAutoDisabled,
				"disabled_reason": ChannelKeyQuotaDisabledReason,
				"disabled_at":     common.GetTimestamp(),
			})
		if statusUpdateResult.Error != nil {
			return result, statusUpdateResult.Error
		}
		result.KeyJustExhausted = statusUpdateResult.RowsAffected == 1
		if result.KeyJustExhausted {
			channelStatusUpdate, err := disableChannelWhenNoUsableKeysTx(tx, channel.Id)
			if err != nil {
				return result, err
			}
			result.ChannelJustExhausted = channelStatusUpdate
		}
	}

	var refreshedUsage ChannelKeyUsage
	if err := tx.Select("key_fingerprint", "key_index", "quota_limit_used", "quota_limit", "status").
		Where("channel_id = ? AND key_fingerprint = ?", channel.Id, fingerprint).
		First(&refreshedUsage).Error; err != nil {
		return result, err
	}

	var refreshedChannel Channel
	if err := tx.Select("status").First(&refreshedChannel, channel.Id).Error; err != nil {
		return result, err
	}

	result.KeyFingerprint = refreshedUsage.KeyFingerprint
	result.KeyIndex = refreshedUsage.KeyIndex
	result.QuotaLimitUsed = refreshedUsage.QuotaLimitUsed
	result.QuotaLimit = refreshedUsage.QuotaLimit
	result.Status = refreshedUsage.Status
	result.ChannelStatus = refreshedChannel.Status
	return result, nil
}

func adjustChannelQuotaUsageTx(tx *gorm.DB, channel *Channel, quotaDelta int) error {
	if tx == nil {
		return errors.New("transaction is required")
	}
	if channel == nil || quotaDelta == 0 {
		return nil
	}
	updates := map[string]interface{}{
		"used_quota": buildSignedNonNegativeExpr("used_quota", int64(quotaDelta)),
	}
	if quotaDelta < 0 || (channel.UsesChannelQuota() && channel.QuotaLimit > 0) {
		updates["quota_limit_used"] = buildSignedNonNegativeExpr("quota_limit_used", int64(quotaDelta))
	}
	if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
		return err
	}
	return tx.First(channel, channel.Id).Error
}

func adjustChannelKeyQuotaUsageTx(tx *gorm.DB, channel *Channel, usage *ChannelKeyUsage, quotaDelta int) error {
	if tx == nil {
		return errors.New("transaction is required")
	}
	if channel == nil || usage == nil || quotaDelta == 0 {
		return nil
	}
	if quotaDelta > 0 && !channel.UsesKeyQuota() {
		return nil
	}
	if err := tx.Model(&ChannelKeyUsage{}).
		Where("channel_id = ? AND key_fingerprint = ?", channel.Id, usage.KeyFingerprint).
		Update("quota_limit_used", buildSignedNonNegativeExpr("quota_limit_used", int64(quotaDelta))).Error; err != nil {
		return err
	}
	return tx.Where("channel_id = ? AND key_fingerprint = ?", channel.Id, usage.KeyFingerprint).First(usage).Error
}

func findChannelKeyUsageByFingerprintTx(tx *gorm.DB, channel *Channel, fingerprint string, keyIndex int) (*ChannelKeyUsage, error) {
	if tx == nil {
		return nil, errors.New("transaction is required")
	}
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	currentUsages, err := ensureChannelKeyUsageRecords(tx, channel)
	if err != nil {
		return nil, err
	}
	if usage, ok := currentUsages[keyIndex]; ok && usage != nil && usage.KeyFingerprint == fingerprint {
		return usage, nil
	}
	for _, usage := range currentUsages {
		if usage != nil && usage.KeyFingerprint == fingerprint {
			return usage, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func syncKeyQuotaStatusAfterDeltaTx(tx *gorm.DB, channel *Channel, usage *ChannelKeyUsage, quotaDelta int, now time.Time) error {
	if tx == nil || channel == nil || usage == nil || quotaDelta == 0 {
		return nil
	}
	nowUnix := now.Unix()
	infoChanged := false
	if quotaDelta > 0 && channel.UsesKeyQuota() && usage.Status == common.ChannelStatusEnabled && usage.IsQuotaExceeded() {
		if err := tx.Model(&ChannelKeyUsage{}).Where("id = ?", usage.Id).Updates(map[string]interface{}{
			"status":          common.ChannelStatusAutoDisabled,
			"disabled_reason": ChannelKeyQuotaDisabledReason,
			"disabled_at":     nowUnix,
			"updated_at":      nowUnix,
		}).Error; err != nil {
			return err
		}
		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		channel.ChannelInfo.MultiKeyStatusList[usage.KeyIndex] = common.ChannelStatusAutoDisabled
		channel.ChannelInfo.MultiKeyDisabledReason[usage.KeyIndex] = ChannelKeyQuotaDisabledReason
		channel.ChannelInfo.MultiKeyDisabledTime[usage.KeyIndex] = nowUnix
		infoChanged = true
		usage.Status = common.ChannelStatusAutoDisabled
		usage.DisabledReason = ChannelKeyQuotaDisabledReason
		usage.DisabledAt = nowUnix
	}
	if quotaDelta < 0 && usage.Status == common.ChannelStatusAutoDisabled && usage.DisabledReason == ChannelKeyQuotaDisabledReason && !usage.IsQuotaExceeded() {
		if err := tx.Model(&ChannelKeyUsage{}).Where("id = ?", usage.Id).Updates(map[string]interface{}{
			"status":          common.ChannelStatusEnabled,
			"disabled_reason": "",
			"disabled_at":     0,
			"updated_at":      nowUnix,
		}).Error; err != nil {
			return err
		}
		delete(channel.ChannelInfo.MultiKeyStatusList, usage.KeyIndex)
		delete(channel.ChannelInfo.MultiKeyDisabledReason, usage.KeyIndex)
		delete(channel.ChannelInfo.MultiKeyDisabledTime, usage.KeyIndex)
		infoChanged = true
		usage.Status = common.ChannelStatusEnabled
		usage.DisabledReason = ""
		usage.DisabledAt = 0
	}
	if !infoChanged {
		return nil
	}
	normalizeChannelInfoStatusMaps(&channel.ChannelInfo)
	return persistChannelQuotaStatusTx(tx, channel)
}

func syncChannelQuotaStatusAfterDeltaTx(tx *gorm.DB, channel *Channel, quotaDelta int, now time.Time) error {
	if tx == nil || channel == nil || quotaDelta == 0 {
		return nil
	}
	nowUnix := now.Unix()
	changed := false
	info := channel.GetOtherInfo()

	if quotaDelta > 0 && channel.Status == common.ChannelStatusEnabled && channel.IsChannelQuotaExceeded() {
		channel.Status = common.ChannelStatusAutoDisabled
		info["status_reason"] = ChannelQuotaDisabledReason
		info["status_time"] = nowUnix
		channel.SetOtherInfo(info)
		changed = true
	}

	if quotaDelta < 0 && channel.Status == common.ChannelStatusAutoDisabled && !channel.IsChannelQuotaExceeded() {
		reason, _ := info["status_reason"].(string)
		if reason == ChannelQuotaDisabledReason || reason == ChannelKeyQuotaDisabledReason || reason == "All keys are disabled" {
			if !channel.ChannelInfo.IsMultiKey || !channel.UsesKeyQuota() {
				channel.Status = common.ChannelStatusEnabled
				delete(info, "status_reason")
				delete(info, "status_time")
				channel.SetOtherInfo(info)
				changed = true
			} else if hasUsableChannelKeyTx(tx, channel.Id) {
				channel.Status = common.ChannelStatusEnabled
				delete(info, "status_reason")
				delete(info, "status_time")
				channel.SetOtherInfo(info)
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return persistChannelQuotaStatusTx(tx, channel)
}

func hasUsableChannelKeyTx(tx *gorm.DB, channelID int) bool {
	var usableKeyCount int64
	if err := tx.Model(&ChannelKeyUsage{}).
		Where("channel_id = ? AND status = ? AND (quota_limit <= 0 OR quota_limit_used < quota_limit)", channelID, common.ChannelStatusEnabled).
		Count(&usableKeyCount).Error; err != nil {
		return false
	}
	return usableKeyCount > 0
}

func persistChannelQuotaStatusTx(tx *gorm.DB, channel *Channel) error {
	if tx == nil || channel == nil {
		return nil
	}
	updates := map[string]interface{}{
		"channel_info": channel.ChannelInfo,
		"status":       channel.Status,
		"other_info":   channel.OtherInfo,
	}
	return tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error
}

func recordSignedChannelUsageDailyTx(tx *gorm.DB, channelID int, keyFingerprint string, quotaDelta int64, tokenUsedDelta int64, requestCountDelta int64, now time.Time) error {
	if tx == nil {
		return errors.New("transaction is required")
	}
	if channelID <= 0 {
		return errors.New("invalid channel id")
	}
	usageDate, err := channelUsageDateFromTime(now)
	if err != nil {
		return err
	}
	updatedAt := now.Unix()
	if err := applySignedChannelUsageDailyRowTx(tx, ChannelUsageDaily{
		ChannelId:      channelID,
		KeyFingerprint: "",
		UsageDate:      usageDate,
		Quota:          quotaDelta,
		TokenUsed:      tokenUsedDelta,
		RequestCount:   requestCountDelta,
		UpdatedAt:      updatedAt,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(keyFingerprint) == "" {
		return nil
	}
	return applySignedChannelUsageDailyRowTx(tx, ChannelUsageDaily{
		ChannelId:      channelID,
		KeyFingerprint: keyFingerprint,
		UsageDate:      usageDate,
		Quota:          quotaDelta,
		TokenUsed:      tokenUsedDelta,
		RequestCount:   requestCountDelta,
		UpdatedAt:      updatedAt,
	})
}

func applySignedChannelUsageDailyRowTx(tx *gorm.DB, delta ChannelUsageDaily) error {
	if delta.Quota == 0 && delta.TokenUsed == 0 && delta.RequestCount == 0 {
		return nil
	}
	lookup := buildChannelUsageDailyLookup(delta)
	updates := map[string]interface{}{
		"quota":         buildSignedNonNegativeExpr("quota", delta.Quota),
		"token_used":    buildSignedNonNegativeExpr("token_used", delta.TokenUsed),
		"request_count": buildSignedNonNegativeExpr("request_count", delta.RequestCount),
		"updated_at":    delta.UpdatedAt,
	}
	result := tx.Model(&ChannelUsageDaily{}).Where(lookup).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	if delta.Quota <= 0 && delta.TokenUsed <= 0 && delta.RequestCount <= 0 {
		return nil
	}
	createDelta := delta
	if createDelta.Quota < 0 {
		createDelta.Quota = 0
	}
	if createDelta.TokenUsed < 0 {
		createDelta.TokenUsed = 0
	}
	if createDelta.RequestCount < 0 {
		createDelta.RequestCount = 0
	}
	createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&createDelta)
	if createResult.Error != nil {
		return createResult.Error
	}
	if createResult.RowsAffected > 0 {
		return nil
	}
	return tx.Model(&ChannelUsageDaily{}).Where(lookup).Updates(updates).Error
}

func buildSignedNonNegativeExpr(column string, delta int64) interface{} {
	if delta >= 0 {
		return gorm.Expr(column+" + ?", delta)
	}
	return gorm.Expr("CASE WHEN "+column+" + ? < 0 THEN 0 ELSE "+column+" + ? END", delta, delta)
}

func disableChannelWhenNoUsableKeysTx(tx *gorm.DB, channelID int) (bool, error) {
	if tx == nil {
		return false, errors.New("transaction is required")
	}

	var usableKeyCount int64
	if err := tx.Model(&ChannelKeyUsage{}).
		Where(
			"channel_id = ? AND status = ? AND (quota_limit <= 0 OR quota_limit_used < quota_limit)",
			channelID,
			common.ChannelStatusEnabled,
		).
		Count(&usableKeyCount).Error; err != nil {
		return false, err
	}
	if usableKeyCount > 0 {
		return false, nil
	}

	var channel Channel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "status", "other_info").
		First(&channel, channelID).Error; err != nil {
		return false, err
	}
	if channel.Status != common.ChannelStatusEnabled {
		return false, nil
	}

	info := channel.GetOtherInfo()
	info["status_reason"] = ChannelKeyQuotaDisabledReason
	info["status_time"] = common.GetTimestamp()
	channel.SetOtherInfo(info)

	statusUpdate := tx.Model(&Channel{}).
		Where("id = ? AND status = ?", channelID, common.ChannelStatusEnabled).
		Updates(map[string]interface{}{
			"status":     common.ChannelStatusAutoDisabled,
			"other_info": channel.OtherInfo,
		})
	if statusUpdate.Error != nil {
		return false, statusUpdate.Error
	}
	return statusUpdate.RowsAffected == 1, nil
}

func recordChannelUsageDailyTx(tx *gorm.DB, channelID int, keyFingerprint string, quota int64, tokenUsed int64, requestCount int64, now time.Time) error {
	if tx == nil {
		return errors.New("transaction is required")
	}
	if channelID <= 0 {
		return errors.New("invalid channel id")
	}
	if now.IsZero() {
		now = time.Now()
	}

	usageDate, err := channelUsageDateFromTime(now)
	if err != nil {
		return err
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

	if _, err := UpsertChannelUsageDaily(tx, summaryDelta); err != nil {
		return err
	}
	if keyFingerprint == "" {
		return nil
	}

	keyDelta := summaryDelta
	keyDelta.KeyFingerprint = keyFingerprint
	_, err = UpsertChannelUsageDaily(tx, keyDelta)
	return err
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

func runChannelUsageTransaction(fn func(tx *gorm.DB) error) error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	if !isSQLiteDialector(DB) {
		return DB.Transaction(fn)
	}

	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = DB.Transaction(fn)
		if err == nil || !isRetryableSQLiteBusyError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}

	return err
}

func isSQLiteDialector(db *gorm.DB) bool {
	return db != nil && strings.EqualFold(db.Dialector.Name(), common.DatabaseTypeSQLite)
}

func isRetryableSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy")
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

func buildChannelUsageStatsAggregateQuery(db *gorm.DB, channelIDs []int, today string, start30d string) *gorm.DB {
	return db.Model(&ChannelUsageDaily{}).
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
		Where("channel_id IN ? AND key_fingerprint = ? AND usage_date >= ? AND usage_date <= ?", channelIDs, "", start30d, today).
		Group("channel_id")
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
	result := buildEmptyChannelKeyFingerprintSecretQuery(db).
		Update("value", generatedSecret)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func readChannelKeyFingerprintSecretFresh(db *gorm.DB) (Option, error) {
	var option Option
	err := buildChannelKeyFingerprintSecretQuery(db).
		First(&option).Error
	return option, err
}

func buildChannelKeyFingerprintSecretQuery(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{NewDB: true}).Where(map[string]interface{}{
		"key": ChannelKeyFingerprintSecretOption,
	})
}

func buildEmptyChannelKeyFingerprintSecretQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&Option{}).Where(map[string]interface{}{
		"key":   ChannelKeyFingerprintSecretOption,
		"value": "",
	})
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
