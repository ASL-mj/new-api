package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareChannelUsageTable(t *testing.T) {
	t.Helper()
	modelTestDBMutex.Lock()

	previousDB := DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	isolatedDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := isolatedDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	DB = isolatedDB
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	require.NoError(t, isolatedDB.AutoMigrate(&Channel{}, &Ability{}))

	t.Cleanup(func() {
		DB = previousDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		_ = sqlDB.Close()
		modelTestDBMutex.Unlock()
	})
}

func prepareChannelUsageMigrationDB(t *testing.T) {
	t.Helper()
	modelTestDBMutex.Lock()

	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	isolatedDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := isolatedDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	DB = isolatedDB
	LOG_DB = isolatedDB
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		_ = sqlDB.Close()
		modelTestDBMutex.Unlock()
	})
}

func TestNormalizeQuotaLimitMode(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "default empty", input: "", want: "none"},
		{name: "none", input: " NONE ", want: "none"},
		{name: "channel", input: " Channel ", want: "channel"},
		{name: "key", input: "KEY", want: "key"},
		{name: "both", input: "both", want: "both"},
		{name: "unknown", input: "invalid", want: "none"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizeQuotaLimitMode(tc.input))
		})
	}
}

func TestQuotaLimitModeUsageSemantics(t *testing.T) {
	testCases := []struct {
		name             string
		mode             string
		usesChannelQuota bool
		usesKeyQuota     bool
	}{
		{name: "none", mode: "none", usesChannelQuota: false, usesKeyQuota: false},
		{name: "channel", mode: "channel", usesChannelQuota: true, usesKeyQuota: false},
		{name: "key", mode: "key", usesChannelQuota: false, usesKeyQuota: true},
		{name: "both", mode: "both", usesChannelQuota: true, usesKeyQuota: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channel := &Channel{QuotaLimitMode: tc.mode}
			assert.Equal(t, tc.usesChannelQuota, channel.UsesChannelQuota())
			assert.Equal(t, tc.usesKeyQuota, channel.UsesKeyQuota())
		})
	}
}

func TestIsChannelQuotaExceededTreatsZeroAsUnlimited(t *testing.T) {
	channel := &Channel{
		QuotaLimitMode: "channel",
		QuotaLimit:     0,
		QuotaLimitUsed: 999999,
	}

	assert.False(t, channel.IsChannelQuotaExceeded())
}

func TestIsChannelQuotaExceededBlocksSchedulingWhenLimitReached(t *testing.T) {
	channel := &Channel{
		QuotaLimitMode: "both",
		QuotaLimit:     100,
		QuotaLimitUsed: 100,
	}

	assert.True(t, channel.IsChannelQuotaExceeded())
}

func TestResetQuotaLimitUsageOnlyClearsUsageAndUpdatesResetTime(t *testing.T) {
	prepareChannelUsageTable(t)

	channel := &Channel{
		Name:              "quota-reset-test",
		Key:               "test-key",
		Status:            common.ChannelStatusAutoDisabled,
		UsedQuota:         321,
		QuotaLimitMode:    "channel",
		QuotaLimit:        500,
		QuotaLimitUsed:    123,
		QuotaLimitResetAt: 1,
		Group:             "default",
		Models:            "gpt-4o-mini",
	}
	require.NoError(t, DB.Create(channel).Error)

	resetAt := int64(1700000000)
	require.NoError(t, channel.ResetQuotaLimitUsage(resetAt))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)

	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	assert.EqualValues(t, 321, reloaded.UsedQuota)
	assert.EqualValues(t, 500, reloaded.QuotaLimit)
	assert.EqualValues(t, 0, reloaded.QuotaLimitUsed)
	assert.EqualValues(t, resetAt, reloaded.QuotaLimitResetAt)
}

func TestChannelUpdatePersistsZeroQuotaLimit(t *testing.T) {
	prepareChannelUsageTable(t)

	channel := &Channel{
		Name:           "quota-update-test",
		Key:            "test-key",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: "channel",
		QuotaLimit:     500,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	require.NoError(t, channel.Insert())

	channel.QuotaLimit = 0
	require.NoError(t, channel.Update())

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.EqualValues(t, 0, reloaded.QuotaLimit)
}

func TestFingerprintChannelKeyUsesStableSecretKeyedHMAC(t *testing.T) {
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "channel-usage-secret"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
	})

	firstKey := "sk-channel-key-alpha"
	secondKey := "sk-channel-key-beta"

	firstFingerprint := FingerprintChannelKey(firstKey)
	assert.Equal(t, firstFingerprint, FingerprintChannelKey(firstKey))
	assert.Equal(t, firstFingerprint, FingerprintChannelKey([]string{secondKey, firstKey}[1]))
	assert.NotEqual(t, firstFingerprint, FingerprintChannelKey(secondKey))
	assert.Len(t, firstFingerprint, 64)
	assert.True(t, strings.IndexFunc(firstFingerprint, func(r rune) bool {
		return !strings.ContainsRune("0123456789abcdef", r)
	}) == -1)
}

func TestMaskChannelKeyAlwaysReturnsMaskedValue(t *testing.T) {
	rawKey := "sk-channel-secret-12345678"

	assert.Equal(t, MaskTokenKey(rawKey), MaskChannelKey(rawKey))
	assert.NotEqual(t, rawKey, MaskChannelKey(rawKey))
	assert.NotContains(t, MaskChannelKey(rawKey), rawKey)
	assert.Equal(t, "****", MaskChannelKey("abcd"))
	assert.Equal(t, "", MaskChannelKey(""))
}

func TestChannelKeyUsageJSONOmitsPlaintextKey(t *testing.T) {
	rawKey := "sk-channel-secret-12345678"
	payload, err := common.Marshal(ChannelKeyUsage{
		ChannelId:      1,
		KeyFingerprint: FingerprintChannelKey(rawKey),
		KeyIndex:       2,
		KeyMask:        MaskChannelKey(rawKey),
	})
	require.NoError(t, err)

	encoded := string(payload)
	assert.Contains(t, encoded, `"key_mask":`)
	assert.Contains(t, encoded, `"key_fingerprint":`)
	assert.NotContains(t, encoded, rawKey)
	assert.NotContains(t, encoded, `"key":`)
}

func TestChannelUsageModelsMigrateAndEnforceUniqueConstraints(t *testing.T) {
	prepareChannelUsageMigrationDB(t)

	require.NoError(t, migrateDB())

	migrator := DB.Migrator()
	assert.True(t, migrator.HasTable(&ChannelKeyUsage{}))
	assert.True(t, migrator.HasTable(&ChannelUsageDaily{}))
	assert.True(t, migrator.HasIndex(&ChannelKeyUsage{}, "idx_channel_key_fingerprint"))
	assert.True(t, migrator.HasIndex(&ChannelUsageDaily{}, "idx_channel_usage_day"))

	keyUsage := &ChannelKeyUsage{
		ChannelId:      100,
		KeyFingerprint: FingerprintChannelKey("sk-usage-primary"),
		KeyIndex:       0,
		KeyMask:        MaskChannelKey("sk-usage-primary"),
		Status:         common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(keyUsage).Error)

	duplicateKeyUsage := &ChannelKeyUsage{
		ChannelId:      keyUsage.ChannelId,
		KeyFingerprint: keyUsage.KeyFingerprint,
		KeyIndex:       1,
		KeyMask:        MaskChannelKey("sk-usage-primary"),
		Status:         common.ChannelStatusEnabled,
	}
	assert.Error(t, DB.Create(duplicateKeyUsage).Error)

	dailyUsage := &ChannelUsageDaily{
		ChannelId:      100,
		KeyFingerprint: "",
		UsageDate:      "2026-08-01",
		Quota:          100,
		TokenUsed:      200,
		RequestCount:   3,
	}
	require.NoError(t, DB.Create(dailyUsage).Error)

	duplicateDailyUsage := &ChannelUsageDaily{
		ChannelId:      dailyUsage.ChannelId,
		KeyFingerprint: dailyUsage.KeyFingerprint,
		UsageDate:      dailyUsage.UsageDate,
		Quota:          1,
	}
	assert.Error(t, DB.Create(duplicateDailyUsage).Error)
}
