package model

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
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

	var resolvedSecret string
	err = DB.Transaction(func(tx *gorm.DB) error {
		option, err := findChannelKeyFingerprintSecretOption(tx)
		switch {
		case err == nil:
			if strings.TrimSpace(option.Value) != "" {
				resolvedSecret = option.Value
				return nil
			}
			return fillEmptyChannelKeyFingerprintSecret(tx, &option, generatedSecret, &resolvedSecret)
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		option = Option{
			Key:   ChannelKeyFingerprintSecretOption,
			Value: generatedSecret,
		}
		if err := tx.Create(&option).Error; err != nil {
			option, lookupErr := findChannelKeyFingerprintSecretOption(tx)
			if lookupErr != nil {
				return err
			}
			if strings.TrimSpace(option.Value) == "" {
				return fillEmptyChannelKeyFingerprintSecret(tx, &option, generatedSecret, &resolvedSecret)
			}
			resolvedSecret = option.Value
			return nil
		}

		resolvedSecret = generatedSecret
		return nil
	})
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(resolvedSecret) == "" {
		return "", fmt.Errorf("%s option is empty", ChannelKeyFingerprintSecretOption)
	}

	cacheChannelKeyFingerprintSecretOption(resolvedSecret)
	return resolvedSecret, nil
}

func fillEmptyChannelKeyFingerprintSecret(tx *gorm.DB, option *Option, generatedSecret string, resolvedSecret *string) error {
	result := tx.Model(&Option{}).
		Where("key = ? AND value = ?", ChannelKeyFingerprintSecretOption, option.Value).
		Update("value", generatedSecret)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		*resolvedSecret = generatedSecret
		return nil
	}

	reloaded, err := findChannelKeyFingerprintSecretOption(tx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(reloaded.Value) == "" {
		return fmt.Errorf("%s option is empty", ChannelKeyFingerprintSecretOption)
	}
	*resolvedSecret = reloaded.Value
	return nil
}

func findChannelKeyFingerprintSecretOption(db *gorm.DB) (Option, error) {
	var option Option
	err := db.Where("key = ?", ChannelKeyFingerprintSecretOption).First(&option).Error
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
