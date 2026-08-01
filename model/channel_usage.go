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

func ApplyChannelUsage(channelID int, quota int) (ChannelUsageApplyResult, error) {
	var result ChannelUsageApplyResult
	if DB == nil {
		return result, errors.New("database not initialized")
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if quota > 0 {
			updateResult := tx.Model(&Channel{}).
				Where("id = ?", channelID).
				Updates(map[string]interface{}{
					"used_quota": gorm.Expr("used_quota + ?", quota),
					"quota_limit_used": gorm.Expr(
						"quota_limit_used + CASE WHEN quota_limit > 0 AND quota_limit_mode IN ? THEN ? ELSE 0 END",
						[]string{ChannelQuotaLimitModeChannel, ChannelQuotaLimitModeBoth},
						quota,
					),
				})
			if updateResult.Error != nil {
				return updateResult.Error
			}
			if updateResult.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}

			statusUpdateResult := tx.Model(&Channel{}).
				Where(
					"id = ? AND status = ? AND quota_limit > 0 AND quota_limit_mode IN ? AND quota_limit_used >= quota_limit",
					channelID,
					common.ChannelStatusEnabled,
					[]string{ChannelQuotaLimitModeChannel, ChannelQuotaLimitModeBoth},
				).
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
