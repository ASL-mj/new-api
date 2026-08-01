package model

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

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

func EnsureChannelKeyUsageRecords(channel *Channel) (map[int]*ChannelKeyUsage, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	if DB == nil {
		return nil, errors.New("database not initialized")
	}

	keys := channel.GetKeys()
	currentUsages := make(map[int]*ChannelKeyUsage, len(keys))
	if len(keys) == 0 {
		return currentUsages, nil
	}

	type currentKeyMeta struct {
		Fingerprint string
		Index       int
		Mask        string
	}

	currentKeys := make([]currentKeyMeta, 0, len(keys))
	for idx, key := range keys {
		fingerprint, err := FingerprintChannelKey(key)
		if err != nil {
			return nil, err
		}
		currentKeys = append(currentKeys, currentKeyMeta{
			Fingerprint: fingerprint,
			Index:       idx,
			Mask:        MaskChannelKey(key),
		})
	}

	var existingUsages []ChannelKeyUsage
	if err := DB.Where("channel_id = ?", channel.Id).Find(&existingUsages).Error; err != nil {
		return nil, err
	}

	existingByFingerprint := make(map[string]*ChannelKeyUsage, len(existingUsages))
	for i := range existingUsages {
		existingByFingerprint[existingUsages[i].KeyFingerprint] = &existingUsages[i]
	}

	now := common.GetTimestamp()
	createQuotaLimit := int64(0)
	if channel.UsesKeyQuota() && channel.QuotaLimit > 0 {
		createQuotaLimit = channel.QuotaLimit
	}

	creates := make([]ChannelKeyUsage, 0)
	for _, currentKey := range currentKeys {
		if usage, ok := existingByFingerprint[currentKey.Fingerprint]; ok {
			currentUsages[currentKey.Index] = usage
			updates := map[string]interface{}{}
			if usage.KeyIndex != currentKey.Index {
				usage.KeyIndex = currentKey.Index
				updates["key_index"] = currentKey.Index
			}
			if usage.KeyMask != currentKey.Mask {
				usage.KeyMask = currentKey.Mask
				updates["key_mask"] = currentKey.Mask
			}
			if len(updates) > 0 {
				updates["updated_at"] = now
				if err := DB.Model(&ChannelKeyUsage{}).
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
		if err := DB.Create(&creates).Error; err != nil {
			return nil, err
		}
		for i := range creates {
			usage := creates[i]
			currentUsages[usage.KeyIndex] = &usage
		}
	}

	return currentUsages, nil
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
