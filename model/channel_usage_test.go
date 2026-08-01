package model

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
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

	require.NoError(t, isolatedDB.AutoMigrate(&Channel{}, &Ability{}, &Option{}, &ChannelKeyUsage{}, &ChannelUsageDaily{}))

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

	require.NoError(t, isolatedDB.AutoMigrate(&Channel{}, &Ability{}, &Option{}, &ChannelKeyUsage{}, &ChannelUsageDaily{}))

	t.Cleanup(func() {
		DB = previousDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		_ = sqlDB.Close()
		modelTestDBMutex.Unlock()
	})
}

func resetBatchUpdateStoresForTest() {
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
}

func getBatchUpdateStoreValueForTest(type_ int, id int) (int, bool) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	value, ok := batchUpdateStores[type_][id]
	return value, ok
}

func resetChannelCacheForTest() {
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	group2model2channels = nil
	channelsIDM = nil
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

func configureChannelUsageSecret(t *testing.T) {
	t.Helper()
	t.Setenv("CRYPTO_SECRET", "channel-usage-secret")
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "channel-usage-secret"
	resetChannelKeyFingerprintSecretCache()
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		resetChannelKeyFingerprintSecretCache()
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

func TestChannelUpdatePreservesCallerMultiKeyStateWhenKeyOrderUnchanged(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	resetChannelCacheForTest()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		resetChannelCacheForTest()
	})

	testCases := []struct {
		name                  string
		initialStatusList     map[int]int
		initialDisabledReason map[int]string
		initialDisabledTime   map[int]int64
		mutate                func(*Channel)
		wantStatusList        map[int]int
		wantDisabledReason    map[int]string
		wantDisabledTime      map[int]int64
	}{
		{
			name: "disable_key",
			mutate: func(channel *Channel) {
				if channel.ChannelInfo.MultiKeyStatusList == nil {
					channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
				}
				channel.ChannelInfo.MultiKeyStatusList[1] = common.ChannelStatusManuallyDisabled
			},
			wantStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled},
		},
		{
			name:                  "enable_key",
			initialStatusList:     map[int]int{0: common.ChannelStatusManuallyDisabled},
			initialDisabledReason: map[int]string{0: "manual"},
			initialDisabledTime:   map[int]int64{0: 100},
			mutate: func(channel *Channel) {
				delete(channel.ChannelInfo.MultiKeyStatusList, 0)
				delete(channel.ChannelInfo.MultiKeyDisabledReason, 0)
				delete(channel.ChannelInfo.MultiKeyDisabledTime, 0)
			},
		},
		{
			name: "disable_all_keys",
			mutate: func(channel *Channel) {
				channel.ChannelInfo.MultiKeyStatusList = map[int]int{
					0: common.ChannelStatusManuallyDisabled,
					1: common.ChannelStatusManuallyDisabled,
				}
			},
			wantStatusList: map[int]int{
				0: common.ChannelStatusManuallyDisabled,
				1: common.ChannelStatusManuallyDisabled,
			},
		},
		{
			name:                  "enable_all_keys",
			initialStatusList:     map[int]int{0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusAutoDisabled},
			initialDisabledReason: map[int]string{0: "manual", 1: "auto"},
			initialDisabledTime:   map[int]int64{0: 111, 1: 222},
			mutate: func(channel *Channel) {
				channel.ChannelInfo.MultiKeyStatusList = map[int]int{}
				channel.ChannelInfo.MultiKeyDisabledReason = map[int]string{}
				channel.ChannelInfo.MultiKeyDisabledTime = map[int]int64{}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channel := &Channel{
				Name:           "update-same-keys-" + tc.name,
				Key:            "sk-alpha\nsk-beta",
				Status:         common.ChannelStatusEnabled,
				QuotaLimitMode: ChannelQuotaLimitModeBoth,
				QuotaLimit:     100,
				Group:          "default",
				Models:         "gpt-4o-mini",
				ChannelInfo: ChannelInfo{
					IsMultiKey:             true,
					MultiKeySize:           2,
					MultiKeyMode:           constant.MultiKeyModePolling,
					MultiKeyStatusList:     tc.initialStatusList,
					MultiKeyDisabledReason: tc.initialDisabledReason,
					MultiKeyDisabledTime:   tc.initialDisabledTime,
				},
			}
			require.NoError(t, channel.Insert())
			_, err := EnsureChannelKeyUsageRecords(channel)
			require.NoError(t, err)
			InitChannelCache()

			current, err := GetChannelById(channel.Id, true)
			require.NoError(t, err)
			tc.mutate(current)
			require.NoError(t, current.Update())

			var reloaded Channel
			require.NoError(t, DB.First(&reloaded, channel.Id).Error)
			assert.Equal(t, tc.wantStatusList, reloaded.ChannelInfo.MultiKeyStatusList)
			assert.Equal(t, tc.wantDisabledReason, reloaded.ChannelInfo.MultiKeyDisabledReason)
			assert.Equal(t, tc.wantDisabledTime, reloaded.ChannelInfo.MultiKeyDisabledTime)

			cached, err := CacheGetChannel(channel.Id)
			require.NoError(t, err)
			assert.Equal(t, reloaded.ChannelInfo.MultiKeyStatusList, cached.ChannelInfo.MultiKeyStatusList)
			assert.Equal(t, reloaded.ChannelInfo.MultiKeyDisabledReason, cached.ChannelInfo.MultiKeyDisabledReason)
			assert.Equal(t, reloaded.ChannelInfo.MultiKeyDisabledTime, cached.ChannelInfo.MultiKeyDisabledTime)
		})
	}
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

func TestUpdateChannelUsedQuotaPropagatesAutoDisableToAbilitiesAndCache(t *testing.T) {
	prepareChannelUsageTable(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	common.MemoryCacheEnabled = true
	common.BatchUpdateEnabled = false
	resetChannelCacheForTest()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		resetChannelCacheForTest()
	})

	channel := &Channel{
		Name:           "channel-usage-propagation",
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
	require.NoError(t, channel.AddAbilities(nil))

	InitChannelCache()
	cachedBefore, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, cachedBefore.Status)
	selectedBefore, err := GetRandomSatisfiedChannel("default", "gpt-4o-mini", 0)
	require.NoError(t, err)
	require.NotNil(t, selectedBefore)
	assert.Equal(t, channel.Id, selectedBefore.Id)

	UpdateChannelUsedQuota(channel.Id, 15)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.EqualValues(t, 105, reloaded.UsedQuota)
	assert.EqualValues(t, 105, reloaded.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.NotEmpty(t, abilities)
	for _, ability := range abilities {
		assert.False(t, ability.Enabled)
	}

	cachedAfter, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, cachedAfter.Status)
	selectedAfter, err := GetRandomSatisfiedChannel("default", "gpt-4o-mini", 0)
	require.NoError(t, err)
	assert.Nil(t, selectedAfter)
}

func TestUpdateChannelUsedQuotaInBatchModeAppliesLimitedChannelsImmediately(t *testing.T) {
	prepareChannelUsageTable(t)

	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	resetBatchUpdateStoresForTest()
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		resetBatchUpdateStoresForTest()
	})

	channel := &Channel{
		Name:           "channel-usage-batch-limited",
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
	require.NoError(t, channel.AddAbilities(nil))

	UpdateChannelUsedQuota(channel.Id, 15)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.EqualValues(t, 105, reloaded.UsedQuota)
	assert.EqualValues(t, 105, reloaded.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.NotEmpty(t, abilities)
	for _, ability := range abilities {
		assert.False(t, ability.Enabled)
	}

	_, queued := getBatchUpdateStoreValueForTest(BatchUpdateTypeChannelUsedQuota, channel.Id)
	assert.False(t, queued)
}

func TestUpdateChannelUsedQuotaInBatchModeKeepsUnlimitedChannelsQueued(t *testing.T) {
	prepareChannelUsageTable(t)

	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	resetBatchUpdateStoresForTest()
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		resetBatchUpdateStoresForTest()
	})

	channel := &Channel{
		Name:           "channel-usage-batch-unlimited",
		Key:            "test-key",
		Status:         common.ChannelStatusEnabled,
		UsedQuota:      10,
		QuotaLimitMode: ChannelQuotaLimitModeNone,
		QuotaLimit:     100,
		QuotaLimitUsed: 4,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	require.NoError(t, DB.Create(channel).Error)

	UpdateChannelUsedQuota(channel.Id, 7)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.EqualValues(t, 10, reloaded.UsedQuota)
	assert.EqualValues(t, 4, reloaded.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)

	queuedValue, queued := getBatchUpdateStoreValueForTest(BatchUpdateTypeChannelUsedQuota, channel.Id)
	require.True(t, queued)
	assert.Equal(t, 7, queuedValue)
}

func TestChannelUsageSQLHelpersAreDialectNeutral(t *testing.T) {
	incrementSQL, incrementArgs := buildChannelQuotaLimitIncrementExpr(7)
	disableSQL, disableArgs := buildChannelUsageAutoDisableCondition(123)

	testCases := []struct {
		name string
		sql  string
	}{
		{name: "increment", sql: incrementSQL},
		{name: "disable", sql: disableSQL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			upperSQL := strings.ToUpper(tc.sql)
			assert.NotContains(t, upperSQL, "RETURNING")
			assert.NotContains(t, upperSQL, "JSONB")
			assert.NotContains(t, tc.sql, "`")
			assert.NotContains(t, tc.sql, "\"")
			assert.NotContains(t, tc.sql, "::")
		})
	}

	assert.Contains(t, incrementSQL, "CASE WHEN")
	assert.Contains(t, incrementSQL, "quota_limit_mode IN ?")
	assert.Equal(t, []interface{}{[]string{ChannelQuotaLimitModeChannel, ChannelQuotaLimitModeBoth}, 7}, incrementArgs)

	assert.Contains(t, disableSQL, "status = ?")
	assert.Contains(t, disableSQL, "quota_limit_used >= quota_limit")
	require.Len(t, disableArgs, 3)
	assert.Equal(t, 123, disableArgs[0])
	assert.Equal(t, common.ChannelStatusEnabled, disableArgs[1])
	assert.Equal(t, []string{ChannelQuotaLimitModeChannel, ChannelQuotaLimitModeBoth}, disableArgs[2])
}

func TestChannelUsageDryRunSQLAcrossDialectors(t *testing.T) {
	prepareChannelUsageTable(t)

	sqlDB, err := DB.DB()
	require.NoError(t, err)

	testCases := []struct {
		name        string
		open        func(*sql.DB) (*gorm.DB, error)
		wantQuote   string
		wantBindVar string
	}{
		{
			name: "sqlite",
			open: func(conn *sql.DB) (*gorm.DB, error) {
				return gorm.Open(sqlite.Dialector{Conn: conn}, &gorm.Config{DryRun: true})
			},
			wantQuote:   "`channels`",
			wantBindVar: "?",
		},
		{
			name: "mysql",
			open: func(conn *sql.DB) (*gorm.DB, error) {
				return gorm.Open(mysql.New(mysql.Config{
					Conn:                      conn,
					SkipInitializeWithVersion: true,
				}), &gorm.Config{DryRun: true})
			},
			wantQuote:   "`channels`",
			wantBindVar: "?",
		},
		{
			name: "postgres",
			open: func(conn *sql.DB) (*gorm.DB, error) {
				return gorm.Open(postgres.New(postgres.Config{
					Conn:             conn,
					WithoutReturning: true,
				}), &gorm.Config{DryRun: true})
			},
			wantQuote:   "\"channels\"",
			wantBindVar: "$",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dryRunDB, err := tc.open(sqlDB)
			require.NoError(t, err)

			updateStmt := dryRunDB.Model(&Channel{}).
				Where("id = ?", 123).
				Updates(buildChannelUsageUpdates(7)).Statement
			updateSQL := updateStmt.SQL.String()
			updateSQLUpper := strings.ToUpper(updateSQL)

			assert.Contains(t, updateSQL, tc.wantQuote)
			assert.Contains(t, updateSQL, "CASE WHEN")
			assert.Contains(t, updateSQLUpper, "UPDATE")
			assert.Contains(t, updateSQLUpper, "SET")
			assert.NotContains(t, updateSQLUpper, "RETURNING")
			assert.NotContains(t, updateSQLUpper, "JSONB")
			assert.Contains(t, updateSQL, tc.wantBindVar)
			assert.NotEmpty(t, updateStmt.Vars)

			disableConditionSQL, disableConditionArgs := buildChannelUsageAutoDisableCondition(123)
			disableStmt := dryRunDB.Model(&Channel{}).
				Where(disableConditionSQL, disableConditionArgs...).
				Update("status", common.ChannelStatusAutoDisabled).Statement
			disableSQL := disableStmt.SQL.String()
			disableSQLUpper := strings.ToUpper(disableSQL)

			assert.Contains(t, disableSQL, tc.wantQuote)
			assert.Contains(t, disableSQLUpper, "QUOTA_LIMIT_USED >= QUOTA_LIMIT")
			assert.NotContains(t, disableSQLUpper, "RETURNING")
			assert.NotContains(t, disableSQLUpper, "JSONB")
			assert.Contains(t, disableSQL, tc.wantBindVar)
			assert.NotEmpty(t, disableStmt.Vars)
		})
	}
}

func TestEnsureChannelKeyUsageRecordsReconcilesByFingerprintWithoutResettingExistingUsage(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	channel := &Channel{
		Name:           "key-usage-reconcile",
		Key:            "sk-alpha\nsk-beta\nsk-gamma",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	require.NoError(t, DB.Create(channel).Error)

	alphaFingerprint, err := FingerprintChannelKey("sk-alpha")
	require.NoError(t, err)
	betaFingerprint, err := FingerprintChannelKey("sk-beta")
	require.NoError(t, err)
	orphanFingerprint, err := FingerprintChannelKey("sk-orphan")
	require.NoError(t, err)

	require.NoError(t, DB.Create([]*ChannelKeyUsage{
		{
			ChannelId:         channel.Id,
			KeyFingerprint:    alphaFingerprint,
			KeyIndex:          0,
			KeyMask:           MaskChannelKey("sk-alpha"),
			QuotaLimit:        123,
			QuotaLimitUsed:    95,
			QuotaLimitResetAt: 111,
			Status:            common.ChannelStatusAutoDisabled,
			DisabledReason:    "already exhausted",
			DisabledAt:        222,
		},
		{
			ChannelId:         channel.Id,
			KeyFingerprint:    betaFingerprint,
			KeyIndex:          1,
			KeyMask:           MaskChannelKey("sk-beta"),
			QuotaLimit:        77,
			QuotaLimitUsed:    40,
			QuotaLimitResetAt: 333,
			Status:            common.ChannelStatusEnabled,
		},
		{
			ChannelId:         channel.Id,
			KeyFingerprint:    orphanFingerprint,
			KeyIndex:          2,
			KeyMask:           MaskChannelKey("sk-orphan"),
			QuotaLimit:        66,
			QuotaLimitUsed:    55,
			QuotaLimitResetAt: 444,
			Status:            common.ChannelStatusAutoDisabled,
			DisabledReason:    "legacy",
			DisabledAt:        555,
		},
	}).Error)

	channel.Key = "sk-gamma\nsk-alpha\nsk-beta"
	channel.ChannelInfo.MultiKeySize = 3

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.Len(t, currentUsages, 3)

	var records []ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&records).Error)
	require.Len(t, records, 4)

	recordsByFingerprint := make(map[string]ChannelKeyUsage, len(records))
	for _, record := range records {
		recordsByFingerprint[record.KeyFingerprint] = record
	}

	alphaRecord := recordsByFingerprint[alphaFingerprint]
	assert.Equal(t, 1, alphaRecord.KeyIndex)
	assert.Equal(t, MaskChannelKey("sk-alpha"), alphaRecord.KeyMask)
	assert.EqualValues(t, 123, alphaRecord.QuotaLimit)
	assert.EqualValues(t, 95, alphaRecord.QuotaLimitUsed)
	assert.EqualValues(t, 111, alphaRecord.QuotaLimitResetAt)
	assert.Equal(t, common.ChannelStatusAutoDisabled, alphaRecord.Status)
	assert.Equal(t, "already exhausted", alphaRecord.DisabledReason)
	assert.EqualValues(t, 222, alphaRecord.DisabledAt)

	betaRecord := recordsByFingerprint[betaFingerprint]
	assert.Equal(t, 2, betaRecord.KeyIndex)
	assert.EqualValues(t, 77, betaRecord.QuotaLimit)
	assert.EqualValues(t, 40, betaRecord.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusEnabled, betaRecord.Status)

	gammaFingerprint, err := FingerprintChannelKey("sk-gamma")
	require.NoError(t, err)
	gammaRecord := recordsByFingerprint[gammaFingerprint]
	assert.Equal(t, 0, gammaRecord.KeyIndex)
	assert.Equal(t, MaskChannelKey("sk-gamma"), gammaRecord.KeyMask)
	assert.EqualValues(t, channel.QuotaLimit, gammaRecord.QuotaLimit)
	assert.EqualValues(t, 0, gammaRecord.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusEnabled, gammaRecord.Status)

	orphanRecord := recordsByFingerprint[orphanFingerprint]
	assert.EqualValues(t, 55, orphanRecord.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusAutoDisabled, orphanRecord.Status)
}

func TestEnsureChannelKeyUsageRecordsRemapsMultiKeyStatusByFingerprintAfterReorder(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	resetChannelCacheForTest()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		resetChannelCacheForTest()
	})

	channel := &Channel{
		Name:           "key-usage-remap-reorder",
		Key:            "sk-alpha\nsk-beta",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
			},
			MultiKeyDisabledReason: map[int]string{
				0: "alpha disabled",
			},
			MultiKeyDisabledTime: map[int]int64{
				0: 12345,
			},
		},
	}
	require.NoError(t, channel.Insert())
	require.NoError(t, channel.AddAbilities(nil))

	_, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)

	channel.Key = "sk-beta\nsk-alpha"
	require.NoError(t, channel.Update())

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.Len(t, currentUsages, 2)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	require.Equal(t, 2, reloaded.ChannelInfo.MultiKeySize)
	require.NotNil(t, reloaded.ChannelInfo.MultiKeyStatusList)
	assert.NotContains(t, reloaded.ChannelInfo.MultiKeyStatusList, 0)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.ChannelInfo.MultiKeyStatusList[1])
	assert.Equal(t, "alpha disabled", reloaded.ChannelInfo.MultiKeyDisabledReason[1])
	assert.EqualValues(t, 12345, reloaded.ChannelInfo.MultiKeyDisabledTime[1])

	alphaFingerprint, err := FingerprintChannelKey("sk-alpha")
	require.NoError(t, err)
	betaFingerprint, err := FingerprintChannelKey("sk-beta")
	require.NoError(t, err)

	var usages []ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("key_index asc").Find(&usages).Error)
	require.Len(t, usages, 2)
	usageByFingerprint := make(map[string]ChannelKeyUsage, len(usages))
	for _, usage := range usages {
		usageByFingerprint[usage.KeyFingerprint] = usage
	}
	assert.Equal(t, 1, usageByFingerprint[alphaFingerprint].KeyIndex)
	assert.Equal(t, 0, usageByFingerprint[betaFingerprint].KeyIndex)

	InitChannelCache()
	cachedChannel, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, reloaded.ChannelInfo.MultiKeyStatusList, cachedChannel.ChannelInfo.MultiKeyStatusList)
	assert.Equal(t, reloaded.ChannelInfo.MultiKeyDisabledReason, cachedChannel.ChannelInfo.MultiKeyDisabledReason)
	assert.Equal(t, reloaded.ChannelInfo.MultiKeyDisabledTime, cachedChannel.ChannelInfo.MultiKeyDisabledTime)

	selectedKey, selectedIndex, apiErr := cachedChannel.GetNextEnabledKey()
	require.Nil(t, apiErr)
	assert.Equal(t, "sk-beta", selectedKey)
	assert.Equal(t, 0, selectedIndex)
}

func TestEnsureChannelKeyUsageRecordsDropsDeletedKeyFromStatusMapButKeepsHistory(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	resetChannelCacheForTest()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		resetChannelCacheForTest()
	})

	channel := &Channel{
		Name:           "key-usage-remap-delete",
		Key:            "sk-alpha\nsk-beta",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
			},
			MultiKeyDisabledReason: map[int]string{
				0: "alpha disabled",
			},
			MultiKeyDisabledTime: map[int]int64{
				0: 67890,
			},
		},
	}
	require.NoError(t, channel.Insert())

	_, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)

	alphaFingerprint, err := FingerprintChannelKey("sk-alpha")
	require.NoError(t, err)
	betaFingerprint, err := FingerprintChannelKey("sk-beta")
	require.NoError(t, err)

	channel.Key = "sk-beta"
	require.NoError(t, channel.Update())

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.Len(t, currentUsages, 1)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, 1, reloaded.ChannelInfo.MultiKeySize)
	assert.Nil(t, reloaded.ChannelInfo.MultiKeyStatusList)
	assert.Nil(t, reloaded.ChannelInfo.MultiKeyDisabledReason)
	assert.Nil(t, reloaded.ChannelInfo.MultiKeyDisabledTime)

	var usages []ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&usages).Error)
	require.Len(t, usages, 2)
	usageByFingerprint := make(map[string]ChannelKeyUsage, len(usages))
	for _, usage := range usages {
		usageByFingerprint[usage.KeyFingerprint] = usage
	}
	assert.Equal(t, 0, usageByFingerprint[betaFingerprint].KeyIndex)
	assert.Equal(t, common.ChannelStatusEnabled, usageByFingerprint[betaFingerprint].Status)
	assert.Equal(t, common.ChannelStatusEnabled, currentUsages[0].Status)
	_, alphaUsageExists := usageByFingerprint[alphaFingerprint]
	assert.True(t, alphaUsageExists)

	InitChannelCache()
	cachedChannel, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Nil(t, cachedChannel.ChannelInfo.MultiKeyStatusList)
	assert.Nil(t, cachedChannel.ChannelInfo.MultiKeyDisabledReason)
	assert.Nil(t, cachedChannel.ChannelInfo.MultiKeyDisabledTime)

	selectedKey, selectedIndex, apiErr := cachedChannel.GetNextEnabledKey()
	require.Nil(t, apiErr)
	assert.Equal(t, "sk-beta", selectedKey)
	assert.Equal(t, 0, selectedIndex)
}

func TestChannelUpdateRollsBackWhenEnsureReconcileFails(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	channel := &Channel{
		Name:           "rollback-before",
		Key:            "sk-alpha",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, channel.Insert())
	_, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)

	callbackName := "test:fail_channel_key_usage_create"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == (&ChannelKeyUsage{}).TableName() {
			tx.AddError(errors.New("injected reconcile failure"))
		}
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Create().Remove(callbackName)
	})

	current, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	current.Name = "rollback-after"
	current.Key = "sk-alpha\nsk-beta"
	current.ChannelInfo.MultiKeySize = 2

	err = current.Update()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected reconcile failure")

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "rollback-before", reloaded.Name)
	assert.Equal(t, "sk-alpha", reloaded.Key)
	assert.Equal(t, 1, reloaded.ChannelInfo.MultiKeySize)

	var usages []ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&usages).Error)
	require.Len(t, usages, 1)
}

func TestEnsureChannelKeyUsageRecordsConcurrentCreatesConvergeWithoutUniqueConflicts(t *testing.T) {
	prepareConcurrentChannelUsageTable(t)
	configureChannelUsageSecret(t)

	channel := &Channel{
		Name:           "ensure-concurrent",
		Key:            "sk-alpha\nsk-beta",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, channel.Insert())

	const goroutineCount = 8
	results := make(chan map[int]*ChannelKeyUsage, goroutineCount)
	errorsCh := make(chan error, goroutineCount)
	start := make(chan struct{})

	var waitGroup sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := ensureChannelKeyUsageRecordsWithRetry(channel)
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
	for result := range results {
		require.Len(t, result, 2)
	}

	var usages []ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&usages).Error)
	require.Len(t, usages, 2)
}

func TestApplyChannelKeyUsageOnlyAccumulatesForKeyModes(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	testCases := []struct {
		name     string
		mode     string
		expected int64
	}{
		{name: "none", mode: ChannelQuotaLimitModeNone, expected: 3},
		{name: "channel", mode: ChannelQuotaLimitModeChannel, expected: 3},
		{name: "key", mode: ChannelQuotaLimitModeKey, expected: 10},
		{name: "both", mode: ChannelQuotaLimitModeBoth, expected: 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channel := &Channel{
				Name:           "key-usage-" + tc.name,
				Key:            "sk-" + tc.name,
				Status:         common.ChannelStatusEnabled,
				QuotaLimitMode: tc.mode,
				QuotaLimit:     100,
				Group:          "default",
				Models:         "gpt-4o-mini",
				ChannelInfo: ChannelInfo{
					IsMultiKey:   true,
					MultiKeySize: 1,
					MultiKeyMode: constant.MultiKeyModeRandom,
				},
			}
			require.NoError(t, DB.Create(channel).Error)

			currentUsages, err := EnsureChannelKeyUsageRecords(channel)
			require.NoError(t, err)
			usage := currentUsages[0]
			require.NotNil(t, usage)
			require.NoError(t, DB.Model(&ChannelKeyUsage{}).
				Where("id = ?", usage.Id).
				Updates(map[string]interface{}{
					"quota_limit":      100,
					"quota_limit_used": 3,
					"status":           common.ChannelStatusEnabled,
				}).Error)

			result, err := ApplyChannelKeyUsage(channel, "sk-"+tc.name, 0, 7)
			require.NoError(t, err)
			assert.EqualValues(t, tc.expected, result.QuotaLimitUsed)
			assert.Equal(t, common.ChannelStatusEnabled, result.Status)
			assert.False(t, result.KeyJustExhausted)

			var reloaded ChannelKeyUsage
			require.NoError(t, DB.First(&reloaded, usage.Id).Error)
			assert.EqualValues(t, tc.expected, reloaded.QuotaLimitUsed)
			assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
		})
	}
}

func TestApplyChannelKeyUsageReturnsFreshStateForNonPositiveQuota(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	channel := &Channel{
		Name:           "key-usage-no-op",
		Key:            "sk-no-op",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     50,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	require.NoError(t, DB.Create(channel).Error)

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	usage := currentUsages[0]
	require.NotNil(t, usage)
	require.NoError(t, DB.Model(&ChannelKeyUsage{}).
		Where("id = ?", usage.Id).
		Updates(map[string]interface{}{
			"quota_limit":      50,
			"quota_limit_used": 3,
			"status":           common.ChannelStatusEnabled,
		}).Error)

	zeroResult, err := ApplyChannelKeyUsage(channel, "sk-no-op", 0, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 3, zeroResult.QuotaLimitUsed)
	assert.EqualValues(t, 50, zeroResult.QuotaLimit)
	assert.Equal(t, common.ChannelStatusEnabled, zeroResult.Status)
	assert.False(t, zeroResult.KeyJustExhausted)

	negativeResult, err := ApplyChannelKeyUsage(channel, "sk-no-op", 0, -5)
	require.NoError(t, err)
	assert.EqualValues(t, 3, negativeResult.QuotaLimitUsed)
	assert.EqualValues(t, 50, negativeResult.QuotaLimit)
	assert.Equal(t, common.ChannelStatusEnabled, negativeResult.Status)
	assert.False(t, negativeResult.KeyJustExhausted)

	var reloaded ChannelKeyUsage
	require.NoError(t, DB.First(&reloaded, usage.Id).Error)
	assert.EqualValues(t, 3, reloaded.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
}

func TestApplyChannelKeyUsageDisablesOnlyExceededKeyAndKeepsSiblingsAvailable(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	resetChannelCacheForTest()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		resetChannelCacheForTest()
	})

	channel := &Channel{
		Name:           "key-usage-single-exhaustion",
		Key:            "sk-alpha\nsk-beta",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ChannelKeyUsage{}).
		Where("id = ?", currentUsages[0].Id).
		Updates(map[string]interface{}{
			"quota_limit":      100,
			"quota_limit_used": 90,
			"status":           common.ChannelStatusEnabled,
		}).Error)
	require.NoError(t, DB.Model(&ChannelKeyUsage{}).
		Where("id = ?", currentUsages[1].Id).
		Updates(map[string]interface{}{
			"quota_limit":      100,
			"quota_limit_used": 0,
			"status":           common.ChannelStatusEnabled,
		}).Error)

	InitChannelCache()
	cachedChannel, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)

	result, err := ApplyChannelKeyUsage(cachedChannel, "sk-alpha", 0, 15)
	require.NoError(t, err)
	assert.EqualValues(t, 105, result.QuotaLimitUsed)
	assert.EqualValues(t, 100, result.QuotaLimit)
	assert.Equal(t, common.ChannelStatusAutoDisabled, result.Status)
	assert.False(t, result.ChannelJustExhausted)
	assert.True(t, result.KeyJustExhausted)

	var alphaUsage ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ? AND key_index = ?", channel.Id, 0).First(&alphaUsage).Error)
	assert.EqualValues(t, 105, alphaUsage.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusAutoDisabled, alphaUsage.Status)

	var betaUsage ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ? AND key_index = ?", channel.Id, 1).First(&betaUsage).Error)
	assert.EqualValues(t, 0, betaUsage.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusEnabled, betaUsage.Status)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
	assert.NotEmpty(t, reloaded.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, reloaded.ChannelInfo.MultiKeyDisabledTime[0])

	cachedAfter, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, cachedAfter.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, cachedAfter.ChannelInfo.MultiKeyStatusList[0])

	selectedKey, selectedIndex, apiErr := cachedAfter.GetNextEnabledKey()
	require.Nil(t, apiErr)
	assert.Equal(t, "sk-beta", selectedKey)
	assert.Equal(t, 1, selectedIndex)

	selectedChannel, err := GetRandomSatisfiedChannel("default", "gpt-4o-mini", 0)
	require.NoError(t, err)
	require.NotNil(t, selectedChannel)
	assert.Equal(t, channel.Id, selectedChannel.Id)
}

func TestApplyChannelKeyUsageDisablesParentChannelWhenLastKeyIsExhausted(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	resetChannelCacheForTest()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		resetChannelCacheForTest()
	})

	channel := &Channel{
		Name:           "key-usage-last-key",
		Key:            "sk-last",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ChannelKeyUsage{}).
		Where("id = ?", currentUsages[0].Id).
		Updates(map[string]interface{}{
			"quota_limit":      100,
			"quota_limit_used": 95,
			"status":           common.ChannelStatusEnabled,
		}).Error)

	InitChannelCache()
	cachedChannel, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)

	result, err := ApplyChannelKeyUsage(cachedChannel, "sk-last", 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 105, result.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusAutoDisabled, result.Status)
	assert.True(t, result.ChannelJustExhausted)
	assert.True(t, result.KeyJustExhausted)

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
	assert.NotEmpty(t, reloaded.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, reloaded.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Contains(t, reloaded.OtherInfo, "All keys are disabled")

	cachedAfter, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, cachedAfter.Status)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.NotEmpty(t, abilities)
	for _, ability := range abilities {
		assert.False(t, ability.Enabled)
	}

	selectedChannel, err := GetRandomSatisfiedChannel("default", "gpt-4o-mini", 0)
	require.NoError(t, err)
	assert.Nil(t, selectedChannel)
}

func TestApplyChannelKeyUsageConcurrentRequestsDoNotLoseUsage(t *testing.T) {
	prepareConcurrentChannelUsageTable(t)
	configureChannelUsageSecret(t)

	channel := &Channel{
		Name:           "key-usage-concurrent",
		Key:            "sk-concurrent",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeKey,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	require.NoError(t, DB.Create(channel).Error)

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ChannelKeyUsage{}).
		Where("id = ?", currentUsages[0].Id).
		Updates(map[string]interface{}{
			"quota_limit":      1000,
			"quota_limit_used": 0,
			"status":           common.ChannelStatusEnabled,
		}).Error)

	const goroutineCount = 10
	const quotaPerRequest = 11

	results := make(chan ChannelKeyUsageApplyResult, goroutineCount)
	errorsCh := make(chan error, goroutineCount)
	start := make(chan struct{})

	var waitGroup sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := applyChannelKeyUsageWithRetry(channel, "sk-concurrent", 0, quotaPerRequest)
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
		if result.KeyJustExhausted {
			exhaustedCount++
		}
	}
	assert.Zero(t, exhaustedCount)

	var reloaded ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ? AND key_index = ?", channel.Id, 0).First(&reloaded).Error)
	assert.EqualValues(t, goroutineCount*quotaPerRequest, reloaded.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
}

func TestApplyChannelKeyUsageConcurrentExhaustionHasSingleWinner(t *testing.T) {
	prepareConcurrentChannelUsageTable(t)
	configureChannelUsageSecret(t)

	channel := &Channel{
		Name:           "key-usage-concurrent-exhaustion",
		Key:            "sk-concurrent-exhaustion",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeKey,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ChannelKeyUsage{}).
		Where("id = ?", currentUsages[0].Id).
		Updates(map[string]interface{}{
			"quota_limit":      100,
			"quota_limit_used": 90,
			"status":           common.ChannelStatusEnabled,
		}).Error)

	const goroutineCount = 2
	const quotaPerRequest = 15

	results := make(chan ChannelKeyUsageApplyResult, goroutineCount)
	errorsCh := make(chan error, goroutineCount)
	start := make(chan struct{})

	var waitGroup sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := applyChannelKeyUsageWithRetry(channel, "sk-concurrent-exhaustion", 0, quotaPerRequest)
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
	channelExhaustedCount := 0
	for result := range results {
		if result.KeyJustExhausted {
			exhaustedCount++
		}
		if result.ChannelJustExhausted {
			channelExhaustedCount++
		}
	}
	assert.Equal(t, 1, exhaustedCount)
	assert.Equal(t, 1, channelExhaustedCount)

	var reloadedUsage ChannelKeyUsage
	require.NoError(t, DB.Where("channel_id = ? AND key_index = ?", channel.Id, 0).First(&reloadedUsage).Error)
	assert.EqualValues(t, 120, reloadedUsage.QuotaLimitUsed)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloadedUsage.Status)

	var reloadedChannel Channel
	require.NoError(t, DB.First(&reloadedChannel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloadedChannel.Status)
}

func TestGetNextEnabledKeySkipsPersistedExceededKeysEvenWhenStatusMapIsStale(t *testing.T) {
	prepareChannelUsageTable(t)
	configureChannelUsageSecret(t)

	channel := &Channel{
		Name:           "key-selection-skip-exhausted",
		Key:            "sk-alpha\nsk-beta",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusEnabled,
				1: common.ChannelStatusEnabled,
			},
		},
	}
	require.NoError(t, DB.Create(channel).Error)

	currentUsages, err := EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ChannelKeyUsage{}).
		Where("id = ?", currentUsages[0].Id).
		Updates(map[string]interface{}{
			"quota_limit":      100,
			"quota_limit_used": 100,
			"status":           common.ChannelStatusEnabled,
		}).Error)
	require.NoError(t, DB.Model(&ChannelKeyUsage{}).
		Where("id = ?", currentUsages[1].Id).
		Updates(map[string]interface{}{
			"quota_limit":      100,
			"quota_limit_used": 0,
			"status":           common.ChannelStatusEnabled,
		}).Error)

	selectedKey, selectedIndex, apiErr := channel.GetNextEnabledKey()
	require.Nil(t, apiErr)
	assert.Equal(t, "sk-beta", selectedKey)
	assert.Equal(t, 1, selectedIndex)
}

func TestGetNextEnabledKeyFailsClosedWhenFingerprintingFails(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "")
	previousSecret := common.CryptoSecret
	previousDB := DB
	common.CryptoSecret = ""
	DB = nil
	resetChannelKeyFingerprintSecretCache()
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		DB = previousDB
		resetChannelKeyFingerprintSecretCache()
	})

	channel := &Channel{
		Key: "sk-alpha\nsk-beta",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}

	selectedKey, selectedIndex, apiErr := channel.GetNextEnabledKey()
	require.NotNil(t, apiErr)
	assert.Empty(t, selectedKey)
	assert.Zero(t, selectedIndex)
}

func TestChannelKeyUsageSQLHelpersAreDialectNeutral(t *testing.T) {
	incrementSQL, incrementArgs := buildChannelKeyUsageQuotaLimitIncrementExpr(7)
	disableSQL, disableArgs := buildChannelKeyUsageAutoDisableCondition(123, "fingerprint")

	testCases := []struct {
		name string
		sql  string
	}{
		{name: "increment", sql: incrementSQL},
		{name: "disable", sql: disableSQL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			upperSQL := strings.ToUpper(tc.sql)
			assert.NotContains(t, upperSQL, "RETURNING")
			assert.NotContains(t, upperSQL, "JSONB")
			assert.NotContains(t, tc.sql, "`")
			assert.NotContains(t, tc.sql, "\"")
			assert.NotContains(t, tc.sql, "::")
		})
	}

	assert.Contains(t, incrementSQL, "CASE WHEN")
	assert.Contains(t, incrementSQL, "quota_limit > 0")
	assert.Equal(t, []interface{}{7}, incrementArgs)

	assert.Contains(t, disableSQL, "status = ?")
	assert.Contains(t, disableSQL, "quota_limit_used >= quota_limit")
	require.Len(t, disableArgs, 3)
	assert.Equal(t, 123, disableArgs[0])
	assert.Equal(t, "fingerprint", disableArgs[1])
	assert.Equal(t, common.ChannelStatusEnabled, disableArgs[2])
}

func TestChannelKeyUsageDryRunSQLAcrossDialectors(t *testing.T) {
	prepareChannelUsageTable(t)

	sqlDB, err := DB.DB()
	require.NoError(t, err)

	testCases := []struct {
		name        string
		open        func(*sql.DB) (*gorm.DB, error)
		wantQuote   string
		wantBindVar string
	}{
		{
			name: "sqlite",
			open: func(conn *sql.DB) (*gorm.DB, error) {
				return gorm.Open(sqlite.Dialector{Conn: conn}, &gorm.Config{DryRun: true})
			},
			wantQuote:   "`channel_key_usages`",
			wantBindVar: "?",
		},
		{
			name: "mysql",
			open: func(conn *sql.DB) (*gorm.DB, error) {
				return gorm.Open(mysql.New(mysql.Config{
					Conn:                      conn,
					SkipInitializeWithVersion: true,
				}), &gorm.Config{DryRun: true})
			},
			wantQuote:   "`channel_key_usages`",
			wantBindVar: "?",
		},
		{
			name: "postgres",
			open: func(conn *sql.DB) (*gorm.DB, error) {
				return gorm.Open(postgres.New(postgres.Config{
					Conn:             conn,
					WithoutReturning: true,
				}), &gorm.Config{DryRun: true})
			},
			wantQuote:   "\"channel_key_usages\"",
			wantBindVar: "$",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dryRunDB, err := tc.open(sqlDB)
			require.NoError(t, err)

			updateStmt := dryRunDB.Model(&ChannelKeyUsage{}).
				Where("channel_id = ? AND key_fingerprint = ?", 123, "fingerprint").
				Updates(buildChannelKeyUsageUpdates(7)).Statement
			updateSQL := updateStmt.SQL.String()
			updateSQLUpper := strings.ToUpper(updateSQL)

			assert.Contains(t, updateSQL, tc.wantQuote)
			assert.Contains(t, updateSQL, "CASE WHEN")
			assert.Contains(t, updateSQLUpper, "UPDATE")
			assert.Contains(t, updateSQLUpper, "SET")
			assert.NotContains(t, updateSQLUpper, "RETURNING")
			assert.NotContains(t, updateSQLUpper, "JSONB")
			assert.Contains(t, updateSQL, tc.wantBindVar)
			assert.NotEmpty(t, updateStmt.Vars)

			disableConditionSQL, disableConditionArgs := buildChannelKeyUsageAutoDisableCondition(123, "fingerprint")
			disableStmt := dryRunDB.Model(&ChannelKeyUsage{}).
				Where(disableConditionSQL, disableConditionArgs...).
				Update("status", common.ChannelStatusAutoDisabled).Statement
			disableSQL := disableStmt.SQL.String()
			disableSQLUpper := strings.ToUpper(disableSQL)

			assert.Contains(t, disableSQL, tc.wantQuote)
			assert.Contains(t, disableSQLUpper, "QUOTA_LIMIT_USED >= QUOTA_LIMIT")
			assert.NotContains(t, disableSQLUpper, "RETURNING")
			assert.NotContains(t, disableSQLUpper, "JSONB")
			assert.Contains(t, disableSQL, tc.wantBindVar)
			assert.NotEmpty(t, disableStmt.Vars)
		})
	}
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

func applyChannelKeyUsageWithRetry(channel *Channel, selectedKey string, keyIndex int, quota int) (ChannelKeyUsageApplyResult, error) {
	var (
		result ChannelKeyUsageApplyResult
		err    error
	)

	for attempt := 0; attempt < 5; attempt++ {
		result, err = ApplyChannelKeyUsage(channel, selectedKey, keyIndex, quota)
		if err == nil || !isSQLiteBusyError(err) {
			return result, err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}

	return result, err
}

func ensureChannelKeyUsageRecordsWithRetry(channel *Channel) (map[int]*ChannelKeyUsage, error) {
	var (
		result map[int]*ChannelKeyUsage
		err    error
	)

	for attempt := 0; attempt < 5; attempt++ {
		result, err = EnsureChannelKeyUsageRecords(channel)
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
