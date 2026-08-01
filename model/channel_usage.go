package model

import "github.com/QuantumNous/new-api/common"

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

func FingerprintChannelKey(key string) string {
	return common.GenerateHMAC(key)
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
