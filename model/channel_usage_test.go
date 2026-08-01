package model

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func prepareConcurrentChannelUsageTable(t *testing.T) {
	t.Helper()
	modelTestDBMutex.Lock()

	previousDB := DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
		filepath.Join(t.TempDir(), "channel-usage-concurrency.db"),
	)
	isolatedDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := isolatedDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(10)

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
		resetChannelKeyFingerprintSecretCache()
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		_ = sqlDB.Close()
		modelTestDBMutex.Unlock()
	})
}

func prepareChannelUsageSecretDB(t *testing.T) {
	t.Helper()
	prepareChannelUsageMigrationDB(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
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

func TestApplyChannelUsageTracksUsedQuotaForNonQuotaModes(t *testing.T) {
	prepareChannelUsageTable(t)

	testCases := []struct {
		name  string
		mode  string
		limit int64
	}{
		{name: "none mode", mode: ChannelQuotaLimitModeNone, limit: 100},
		{name: "key mode", mode: ChannelQuotaLimitModeKey, limit: 100},
		{name: "channel mode with unlimited quota", mode: ChannelQuotaLimitModeChannel, limit: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channel := &Channel{
				Name:           tc.name,
				Key:            "test-key",
				Status:         common.ChannelStatusEnabled,
				UsedQuota:      10,
				QuotaLimitMode: tc.mode,
				QuotaLimit:     tc.limit,
				QuotaLimitUsed: 4,
				Group:          "default",
				Models:         "gpt-4o-mini",
			}
			require.NoError(t, DB.Create(channel).Error)

			result, err := ApplyChannelUsage(channel.Id, 7)
			require.NoError(t, err)

			assert.EqualValues(t, 17, result.UsedQuota)
			assert.EqualValues(t, 4, result.QuotaLimitUsed)
			assert.EqualValues(t, tc.limit, result.QuotaLimit)
			assert.Equal(t, common.ChannelStatusEnabled, result.Status)
			assert.False(t, result.ChannelJustExhausted)
		})
	}
}

func TestApplyChannelUsageReturnsFreshStateForNonPositiveQuota(t *testing.T) {
	prepareChannelUsageTable(t)

	channel := &Channel{
		Name:           "channel-usage-no-op",
		Key:            "test-key",
		Status:         common.ChannelStatusEnabled,
		UsedQuota:      7,
		QuotaLimitMode: ChannelQuotaLimitModeChannel,
		QuotaLimit:     50,
		QuotaLimitUsed: 3,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	require.NoError(t, DB.Create(channel).Error)

	zeroResult, err := ApplyChannelUsage(channel.Id, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 7, zeroResult.UsedQuota)
	assert.EqualValues(t, 3, zeroResult.QuotaLimitUsed)
	assert.EqualValues(t, 50, zeroResult.QuotaLimit)
	assert.Equal(t, common.ChannelStatusEnabled, zeroResult.Status)
	assert.False(t, zeroResult.ChannelJustExhausted)

	negativeResult, err := ApplyChannelUsage(channel.Id, -5)
	require.NoError(t, err)
	assert.EqualValues(t, 7, negativeResult.UsedQuota)
	assert.EqualValues(t, 3, negativeResult.QuotaLimitUsed)
	assert.EqualValues(t, 50, negativeResult.QuotaLimit)
	assert.Equal(t, common.ChannelStatusEnabled, negativeResult.Status)
	assert.False(t, negativeResult.ChannelJustExhausted)
}

func TestApplyChannelUsageCrossesQuotaAndDisablesChannelAfterFullAccounting(t *testing.T) {
	prepareChannelUsageTable(t)

	channel := &Channel{
		Name:           "channel-usage-cross-limit",
		Key:            "test-key",
		Status:         common.ChannelStatusEnabled,
		UsedQuota:      90,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		QuotaLimitUsed: 90,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	require.NoError(t, DB.Create(channel).Error)

	firstResult, err := ApplyChannelUsage(channel.Id, 15)
	require.NoError(t, err)
	assert.EqualValues(t, 105, firstResult.UsedQuota)
	assert.EqualValues(t, 105, firstResult.QuotaLimitUsed)
	assert.EqualValues(t, 100, firstResult.QuotaLimit)
	assert.Equal(t, common.ChannelStatusAutoDisabled, firstResult.Status)
	assert.True(t, firstResult.ChannelJustExhausted)

	secondResult, err := ApplyChannelUsage(channel.Id, 15)
	require.NoError(t, err)
	assert.EqualValues(t, 120, secondResult.UsedQuota)
	assert.EqualValues(t, 120, secondResult.QuotaLimitUsed)
	assert.EqualValues(t, 100, secondResult.QuotaLimit)
	assert.Equal(t, common.ChannelStatusAutoDisabled, secondResult.Status)
	assert.False(t, secondResult.ChannelJustExhausted)
}

func TestApplyChannelUsageConcurrentRequestsDoNotLoseUsage(t *testing.T) {
	prepareConcurrentChannelUsageTable(t)

	channel := &Channel{
		Name:           "channel-usage-concurrent",
		Key:            "test-key",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeChannel,
		QuotaLimit:     1000,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	require.NoError(t, DB.Create(channel).Error)

	const goroutineCount = 10
	const quotaPerRequest = 11

	results := make(chan ChannelUsageApplyResult, goroutineCount)
	errorsCh := make(chan error, goroutineCount)
	start := make(chan struct{})

	var waitGroup sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := applyChannelUsageWithRetry(channel.Id, quotaPerRequest)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result
		}()
	}

	close(start)
	waitGroup.Wait()
	close(results)
	close(errorsCh)

	for err := range errorsCh {
		require.NoError(t, err)
	}

	require.Len(t, results, goroutineCount)
	exhaustedCount := 0
	for result := range results {
		if result.ChannelJustExhausted {
			exhaustedCount++
		}
	}
	assert.Zero(t, exhaustedCount)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.EqualValues(t, goroutineCount*quotaPerRequest, reloaded.UsedQuota)
	assert.EqualValues(t, goroutineCount*quotaPerRequest, reloaded.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
}

func TestApplyChannelUsageConcurrentExhaustionHasSingleWinner(t *testing.T) {
	prepareConcurrentChannelUsageTable(t)

	channel := &Channel{
		Name:           "channel-usage-concurrent-exhaustion",
		Key:            "test-key",
		Status:         common.ChannelStatusEnabled,
		UsedQuota:      90,
		QuotaLimitMode: ChannelQuotaLimitModeChannel,
		QuotaLimit:     100,
		QuotaLimitUsed: 90,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	require.NoError(t, DB.Create(channel).Error)

	const goroutineCount = 2
	const quotaPerRequest = 15

	results := make(chan ChannelUsageApplyResult, goroutineCount)
	errorsCh := make(chan error, goroutineCount)
	start := make(chan struct{})

	var waitGroup sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := applyChannelUsageWithRetry(channel.Id, quotaPerRequest)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result
		}()
	}

	close(start)
	waitGroup.Wait()
	close(results)
	close(errorsCh)

	for err := range errorsCh {
		require.NoError(t, err)
	}

	require.Len(t, results, goroutineCount)
	exhaustedCount := 0
	for result := range results {
		if result.ChannelJustExhausted {
			exhaustedCount++
		}
	}
	assert.Equal(t, 1, exhaustedCount)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.EqualValues(t, 120, reloaded.UsedQuota)
	assert.EqualValues(t, 120, reloaded.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
}

func applyChannelUsageWithRetry(channelID int, quota int) (ChannelUsageApplyResult, error) {
	var (
		result ChannelUsageApplyResult
		err    error
	)

	for attempt := 0; attempt < 5; attempt++ {
		result, err = ApplyChannelUsage(channelID, quota)
		if err == nil || !isSQLiteBusyError(err) {
			return result, err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}

	return result, err
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy")
}

func TestFingerprintChannelKeyUsesStableSecretKeyedHMAC(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "channel-usage-secret")
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "channel-usage-secret"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		resetChannelKeyFingerprintSecretCache()
	})

	firstKey := "sk-channel-key-alpha"
	secondKey := "sk-channel-key-beta"

	firstFingerprint, err := FingerprintChannelKey(firstKey)
	require.NoError(t, err)
	repeatedFingerprint, err := FingerprintChannelKey(firstKey)
	require.NoError(t, err)
	reorderedFingerprint, err := FingerprintChannelKey([]string{secondKey, firstKey}[1])
	require.NoError(t, err)
	secondFingerprint, err := FingerprintChannelKey(secondKey)
	require.NoError(t, err)

	assert.Equal(t, firstFingerprint, repeatedFingerprint)
	assert.Equal(t, firstFingerprint, reorderedFingerprint)
	assert.NotEqual(t, firstFingerprint, secondFingerprint)
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
	t.Setenv("CRYPTO_SECRET", "channel-usage-secret")
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "channel-usage-secret"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		resetChannelKeyFingerprintSecretCache()
	})

	rawKey := "sk-channel-secret-12345678"
	fingerprint, err := FingerprintChannelKey(rawKey)
	require.NoError(t, err)
	payload, err := common.Marshal(ChannelKeyUsage{
		ChannelId:      1,
		KeyFingerprint: fingerprint,
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
	t.Setenv("CRYPTO_SECRET", "")

	require.NoError(t, migrateDB())

	migrator := DB.Migrator()
	assert.True(t, migrator.HasTable(&Option{}))
	assert.True(t, migrator.HasTable(&ChannelKeyUsage{}))
	assert.True(t, migrator.HasTable(&ChannelUsageDaily{}))
	assert.True(t, migrator.HasIndex(&ChannelKeyUsage{}, "idx_channel_key_fingerprint"))
	assert.True(t, migrator.HasIndex(&ChannelUsageDaily{}, "idx_channel_usage_day"))

	fingerprint, err := FingerprintChannelKey("sk-usage-primary")
	require.NoError(t, err)

	keyUsage := &ChannelKeyUsage{
		ChannelId:      100,
		KeyFingerprint: fingerprint,
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

func TestFingerprintChannelKeyPersistsSecretAcrossCacheReset(t *testing.T) {
	prepareChannelUsageSecretDB(t)
	t.Setenv("CRYPTO_SECRET", "")

	previousSecret := common.CryptoSecret
	common.CryptoSecret = "process-random-one"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		resetChannelKeyFingerprintSecretCache()
	})

	firstFingerprint, err := FingerprintChannelKey("sk-persisted-key")
	require.NoError(t, err)

	var stored Option
	require.NoError(t, DB.Where("key = ?", ChannelKeyFingerprintSecretOption).First(&stored).Error)
	require.NotEmpty(t, stored.Value)

	resetChannelKeyFingerprintSecretCache()
	common.CryptoSecret = "process-random-two"

	secondFingerprint, err := FingerprintChannelKey("sk-persisted-key")
	require.NoError(t, err)
	assert.Equal(t, firstFingerprint, secondFingerprint)

	var count int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", ChannelKeyFingerprintSecretOption).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestFingerprintChannelKeyRepairsEmptyOptionAndReadsCommittedValue(t *testing.T) {
	prepareChannelUsageSecretDB(t)
	t.Setenv("CRYPTO_SECRET", "")

	previousSecret := common.CryptoSecret
	common.CryptoSecret = "process-random-empty"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		resetChannelKeyFingerprintSecretCache()
	})

	require.NoError(t, DB.Create(&Option{
		Key:   ChannelKeyFingerprintSecretOption,
		Value: "",
	}).Error)

	// The implementation must repair the empty row and then re-read through a fresh
	// session instead of trusting any stale in-process/transaction snapshot.
	firstFingerprint, err := FingerprintChannelKey("sk-empty-secret-key")
	require.NoError(t, err)

	var stored Option
	require.NoError(t, DB.Where("key = ?", ChannelKeyFingerprintSecretOption).First(&stored).Error)
	require.NotEmpty(t, stored.Value)

	resetChannelKeyFingerprintSecretCache()
	common.CryptoSecret = "process-random-empty-2"

	secondFingerprint, err := FingerprintChannelKey("sk-empty-secret-key")
	require.NoError(t, err)
	assert.Equal(t, firstFingerprint, secondFingerprint)
}

func TestGetChannelKeyFingerprintSecretConcurrentFirstCreate(t *testing.T) {
	prepareChannelUsageSecretDB(t)
	t.Setenv("CRYPTO_SECRET", "")

	resetChannelKeyFingerprintSecretCache()

	const goroutineCount = 16
	results := make(chan string, goroutineCount)
	errorsCh := make(chan error, goroutineCount)
	var waitGroup sync.WaitGroup

	for i := 0; i < goroutineCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			fingerprint, err := FingerprintChannelKey("sk-concurrent-key")
			if err != nil {
				errorsCh <- err
				return
			}
			results <- fingerprint
		}()
	}

	waitGroup.Wait()
	close(results)
	close(errorsCh)

	for err := range errorsCh {
		require.NoError(t, err)
	}

	var fingerprints []string
	for fingerprint := range results {
		fingerprints = append(fingerprints, fingerprint)
	}
	require.Len(t, fingerprints, goroutineCount)
	for _, fingerprint := range fingerprints[1:] {
		assert.Equal(t, fingerprints[0], fingerprint)
	}

	var options []Option
	require.NoError(t, DB.Where("key = ?", ChannelKeyFingerprintSecretOption).Find(&options).Error)
	require.Len(t, options, 1)
	assert.NotEmpty(t, options[0].Value)
}

func TestFingerprintChannelKeyReturnsErrorWhenSecretUnavailable(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "")
	resetChannelKeyFingerprintSecretCache()

	previousDB := DB
	DB = nil
	t.Cleanup(func() {
		DB = previousDB
		resetChannelKeyFingerprintSecretCache()
	})

	fingerprint, err := FingerprintChannelKey("sk-no-db")
	require.Error(t, err)
	assert.Empty(t, fingerprint)
}
