package service

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func channelUsageDateForServiceTest(at time.Time) string {
	tz := common.ChannelUsageTimezone
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return at.In(loc).Format("2006-01-02")
}

func seedChannelUsageTestChannel(t *testing.T, channel *model.Channel) {
	t.Helper()
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
}

func getChannelUsageDailyRow(t *testing.T, channelID int, keyFingerprint string, usageDate string) model.ChannelUsageDaily {
	t.Helper()
	var row model.ChannelUsageDaily
	require.NoError(t, model.DB.Where("channel_id = ? AND key_fingerprint = ? AND usage_date = ?", channelID, keyFingerprint, usageDate).First(&row).Error)
	return row
}

func TestRecordChannelUsageSingleKeyUpdatesChannelKeyAndDaily(t *testing.T) {
	truncate(t)

	channel := &model.Channel{
		Id:             101,
		Name:           "single-key",
		Key:            "sk-single",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	seedChannelUsageTestChannel(t, channel)

	when := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	err := RecordChannelUsage(ChannelUsageRecordParams{
		ChannelID:      channel.Id,
		SelectedKey:    "sk-single",
		KeyIndex:       0,
		HasKeyIdentity: true,
		Quota:          30,
		TokenUsed:      45,
		RequestCount:   1,
		Now:            when,
		ModelName:      "gpt-4o-mini",
		Group:          "default",
		RequestID:      "req-single",
	})
	require.NoError(t, err)

	var reloaded model.Channel
	require.NoError(t, model.DB.First(&reloaded, channel.Id).Error)
	assert.EqualValues(t, 30, reloaded.UsedQuota)
	assert.EqualValues(t, 30, reloaded.QuotaLimitUsed)

	usages, err := model.EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.Len(t, usages, 1)

	var keyUsage model.ChannelKeyUsage
	require.NoError(t, model.DB.Where("channel_id = ? AND key_index = ?", channel.Id, 0).First(&keyUsage).Error)
	assert.EqualValues(t, 30, keyUsage.QuotaLimitUsed)

	usageDate := channelUsageDateForServiceTest(when)
	summary := getChannelUsageDailyRow(t, channel.Id, "", usageDate)
	assert.EqualValues(t, 30, summary.Quota)
	assert.EqualValues(t, 45, summary.TokenUsed)
	assert.EqualValues(t, 1, summary.RequestCount)

	detail := getChannelUsageDailyRow(t, channel.Id, keyUsage.KeyFingerprint, usageDate)
	assert.EqualValues(t, 30, detail.Quota)
	assert.EqualValues(t, 45, detail.TokenUsed)
	assert.EqualValues(t, 1, detail.RequestCount)
}

func TestRecordChannelUsageMultiKeyFirstExhaustionEmitsSingleKeyEvent(t *testing.T) {
	truncate(t)

	channel := &model.Channel{
		Id:             102,
		Name:           "multi-key",
		Key:            "sk-alpha\nsk-beta",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	seedChannelUsageTestChannel(t, channel)

	usages, err := model.EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ChannelKeyUsage{}).
		Where("id = ?", usages[0].Id).
		Updates(map[string]interface{}{
			"quota_limit":      100,
			"quota_limit_used": 95,
			"status":           common.ChannelStatusEnabled,
		}).Error)

	var events []model.SystemEventLog
	previousRecorder := recordChannelUsageSystemEvent
	recordChannelUsageSystemEvent = func(event model.SystemEventLog) {
		events = append(events, event)
	}
	t.Cleanup(func() {
		recordChannelUsageSystemEvent = previousRecorder
	})

	err = RecordChannelUsage(ChannelUsageRecordParams{
		ChannelID:      channel.Id,
		SelectedKey:    "sk-alpha",
		KeyIndex:       0,
		HasKeyIdentity: true,
		Quota:          10,
		TokenUsed:      20,
		RequestCount:   1,
		ModelName:      "gpt-4o-mini",
		Group:          "default",
		RequestID:      "req-multi",
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "channel_usage", events[0].Component)
	assert.Contains(t, events[0].Message, "Key")

	var reloaded model.Channel
	require.NoError(t, model.DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
}

func TestRecordChannelUsageWithoutKeyOnlyWritesChannelSummary(t *testing.T) {
	truncate(t)

	channel := &model.Channel{
		Id:             103,
		Name:           "no-key-context",
		Key:            "sk-task",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeChannel,
		QuotaLimit:     200,
		Group:          "default",
		Models:         "mj",
	}
	seedChannelUsageTestChannel(t, channel)

	when := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)
	err := RecordChannelUsage(ChannelUsageRecordParams{
		ChannelID:      channel.Id,
		Quota:          25,
		RequestCount:   1,
		Now:            when,
		ModelName:      "mj",
		Group:          "default",
		HasKeyIdentity: false,
	})
	require.NoError(t, err)

	var keyUsageCount int64
	require.NoError(t, model.DB.Model(&model.ChannelKeyUsage{}).Where("channel_id = ?", channel.Id).Count(&keyUsageCount).Error)
	assert.Zero(t, keyUsageCount)

	usageDate := channelUsageDateForServiceTest(when)
	summary := getChannelUsageDailyRow(t, channel.Id, "", usageDate)
	assert.EqualValues(t, 25, summary.Quota)
	assert.EqualValues(t, 1, summary.RequestCount)

	var detailCount int64
	require.NoError(t, model.DB.Model(&model.ChannelUsageDaily{}).Where("channel_id = ? AND key_fingerprint <> ''", channel.Id).Count(&detailCount).Error)
	assert.Zero(t, detailCount)
}

func TestRecordChannelUsageRollsBackOnDailyFailure(t *testing.T) {
	truncate(t)

	channel := &model.Channel{
		Id:             104,
		Name:           "rollback",
		Key:            "sk-rollback",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	seedChannelUsageTestChannel(t, channel)

	callbackName := "test:channel_usage_daily_failure"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == (&model.ChannelUsageDaily{}).TableName() {
			tx.AddError(errors.New("injected daily failure"))
		}
	}))
	t.Cleanup(func() {
		_ = model.DB.Callback().Create().Remove(callbackName)
	})

	err := RecordChannelUsage(ChannelUsageRecordParams{
		ChannelID:      channel.Id,
		SelectedKey:    "sk-rollback",
		KeyIndex:       0,
		HasKeyIdentity: true,
		Quota:          20,
		TokenUsed:      30,
		RequestCount:   1,
		ModelName:      "gpt-4o-mini",
		Group:          "default",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected daily failure")

	var reloaded model.Channel
	require.NoError(t, model.DB.First(&reloaded, channel.Id).Error)
	assert.Zero(t, reloaded.UsedQuota)
	assert.Zero(t, reloaded.QuotaLimitUsed)

	var dailyCount int64
	require.NoError(t, model.DB.Model(&model.ChannelUsageDaily{}).Where("channel_id = ?", channel.Id).Count(&dailyCount).Error)
	assert.Zero(t, dailyCount)
}

func TestRecordChannelUsageSQLiteRetrySurvivesConcurrentWrites(t *testing.T) {
	truncate(t)

	channel := &model.Channel{
		Id:             105,
		Name:           "sqlite-retry",
		Key:            "sk-concurrent",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeBoth,
		QuotaLimit:     1000,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	seedChannelUsageTestChannel(t, channel)

	const goroutineCount = 8
	const quotaPerWrite = 11

	start := make(chan struct{})
	errorsCh := make(chan error, goroutineCount)
	var waitGroup sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			errorsCh <- RecordChannelUsage(ChannelUsageRecordParams{
				ChannelID:      channel.Id,
				SelectedKey:    "sk-concurrent",
				KeyIndex:       0,
				HasKeyIdentity: true,
				Quota:          quotaPerWrite,
				TokenUsed:      quotaPerWrite,
				RequestCount:   1,
				ModelName:      "gpt-4o-mini",
				Group:          "default",
			})
		}()
	}

	close(start)
	waitGroup.Wait()
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}

	var reloaded model.Channel
	require.NoError(t, model.DB.First(&reloaded, channel.Id).Error)
	assert.EqualValues(t, goroutineCount*quotaPerWrite, reloaded.UsedQuota)
	assert.EqualValues(t, goroutineCount*quotaPerWrite, reloaded.QuotaLimitUsed)
}

func TestRecordRelayChannelUsageUsesRelayIdentity(t *testing.T) {
	truncate(t)

	channel := &model.Channel{
		Id:             106,
		Name:           "relay-identity",
		Key:            "sk-relay",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeBoth,
		QuotaLimit:     100,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	seedChannelUsageTestChannel(t, channel)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "gpt-4o-mini",
		UsingGroup:      "default",
		RequestId:       "req-relay",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            channel.Id,
			ApiKey:               "sk-relay",
			ChannelMultiKeyIndex: 0,
		},
	}

	err := RecordRelayChannelUsage(relayInfo, 12, 18, 1)
	require.NoError(t, err)

	var keyUsage model.ChannelKeyUsage
	require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).First(&keyUsage).Error)
	assert.EqualValues(t, 12, keyUsage.QuotaLimitUsed)
}
