package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSettleTaskTerminalWalletAndDuplicate(t *testing.T) {
	truncate(t)
	seedUser(t, 101, 1000)
	task := &model.Task{TaskID: "task-settle-wallet", UserId: 101, Quota: 300, Status: model.TaskStatusInProgress, LastPollAt: 10, NextPollAt: 20, SettlementStatus: model.TaskSettlementStatusPending}
	require.NoError(t, model.DB.Create(task).Error)
	task.Status = model.TaskStatusSuccess
	require.NoError(t, SettleTaskTerminal(context.Background(), task, model.TaskStatusInProgress, 200, 200, 7, "success"))
	require.NoError(t, SettleTaskTerminal(context.Background(), task, model.TaskStatusInProgress, 200, 200, 7, "success"))
	var user model.User
	require.NoError(t, model.DB.First(&user, 101).Error)
	require.Equal(t, 1100, user.Quota)
	var count int64
	require.NoError(t, model.DB.Model(&model.TaskSettlement{}).Where("task_id = ?", task.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	require.Equal(t, int64(0), stored.NextPollAt)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stored.Status)
	require.Equal(t, model.TaskSettlementStatus("applied"), stored.SettlementStatus)
	require.Equal(t, 200, stored.Quota)
}

func settlementFixture(t *testing.T, subscription bool) *model.Task {
	t.Helper()
	truncate(t)
	seedUser(t, 201, 1000)
	seedToken(t, 201, 201, "test-secret-must-stay-out-of-outbox", 1000)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 201).Update("used_quota", 300).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 201).Update("used_quota", 300).Error)
	ch := &model.Channel{Id: 201, Name: "settlement", Status: common.ChannelStatusEnabled, Key: "fixture-channel-key", Group: "default", Models: "video", QuotaLimitMode: model.ChannelQuotaLimitModeBoth, QuotaLimit: 10000}
	seedTaskBillingUsageChannel(t, ch)
	task := makeTask(201, 201, 300, 201, BillingSourceWallet, 0)
	if subscription {
		seedSubscription(t, 201, 201, 10000, 300)
		task.PrivateData.BillingSource = BillingSourceSubscription
		task.PrivateData.SubscriptionId = 201
	}
	task.Status, task.PollingVersion = model.TaskStatusInProgress, 2
	task.LastPollAt, task.NextPollAt = 10, 20
	task.PrivateData.ChannelKeyFingerprint = applyTaskPreconsume(t, time.Unix(1700000000, 0), task, ch.Key, 0)
	task.PrivateData.ChannelStandardQuota = 300
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func TestTerminalSettlementConcurrentWalletAndChannel(t *testing.T) {
	task := settlementFixture(t, false)
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copy := *task
			copy.Status = model.TaskStatusSuccess
			errors <- SettleTaskTerminal(context.Background(), &copy, model.TaskStatusInProgress, 200, 200, 10, "provider_terminal")
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, 1100, getUserQuota(t, 201))
	require.Equal(t, 1100, getTokenRemainQuota(t, 201))
	channel := getChannelQuotaState(t, 201)
	require.EqualValues(t, 200, channel.QuotaLimitUsed)
	var row model.TaskSettlement
	require.NoError(t, model.DB.Where("task_id = ?", task.ID).First(&row).Error)
	require.NotContains(t, row.EffectsPayload, "test-secret")
	require.NotContains(t, row.EffectsPayload, "fixture-channel-key")
	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("task_settlement_id = ?", row.ID).Count(&logs).Error)
	require.EqualValues(t, 1, logs)
}

func TestTerminalSettlementSubscriptionRefund(t *testing.T) {
	task := settlementFixture(t, true)
	task.Status = model.TaskStatusFailure
	require.NoError(t, SettleTaskTerminal(context.Background(), task, model.TaskStatusInProgress, 0, 0, 0, "provider_terminal"))
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, 201).Error)
	require.Zero(t, sub.AmountUsed)
	require.Equal(t, 1000, getUserQuota(t, 201))
	require.Equal(t, 1300, getTokenRemainQuota(t, 201))
	require.Zero(t, getChannelQuotaState(t, 201).QuotaLimitUsed)
}

func TestTerminalSettlementRollsBackAllWrites(t *testing.T) {
	task := settlementFixture(t, false)
	const callback = "test:terminal_channel_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(errors.New("injected channel failure"))
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callback) })
	task.Status = model.TaskStatusSuccess
	require.Error(t, SettleTaskTerminal(context.Background(), task, model.TaskStatusInProgress, 200, 200, 0, "provider_terminal"))
	require.Equal(t, 1000, getUserQuota(t, 201))
	require.Equal(t, 1000, getTokenRemainQuota(t, 201))
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	require.EqualValues(t, model.TaskStatusInProgress, stored.Status)
	require.EqualValues(t, model.TaskSettlementStatusPending, stored.SettlementStatus)
	var rows int64
	require.NoError(t, model.DB.Model(&model.TaskSettlement{}).Count(&rows).Error)
	require.Zero(t, rows)
	require.NoError(t, model.DB.Callback().Update().Remove(callback))
	require.NoError(t, SettleTaskTerminal(context.Background(), task, model.TaskStatusInProgress, 200, 200, 0, "provider_terminal"))
	require.Equal(t, 1100, getUserQuota(t, 201))
}

func TestTerminalSettlementLogFailureRetriesWithoutRebilling(t *testing.T) {
	task := settlementFixture(t, false)
	const callback = "test:terminal_log_failure"
	require.NoError(t, model.LOG_DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			tx.AddError(errors.New("injected log failure"))
		}
	}))
	t.Cleanup(func() { _ = model.LOG_DB.Callback().Create().Remove(callback) })
	task.Status = model.TaskStatusSuccess
	require.Error(t, SettleTaskTerminal(context.Background(), task, model.TaskStatusInProgress, 200, 200, 0, "provider_terminal"))
	require.Equal(t, 1100, getUserQuota(t, 201))
	var row model.TaskSettlement
	require.NoError(t, model.DB.Where("task_id = ?", task.ID).First(&row).Error)
	require.Equal(t, model.TaskSettlementStateApplied, row.State)
	require.Zero(t, row.EffectsAppliedAt)
	require.NoError(t, model.LOG_DB.Callback().Create().Remove(callback))
	require.NoError(t, model.DB.Model(&row).Update("effects_next_at", 0).Error)
	require.NoError(t, RetryTaskSettlementEffects(context.Background(), 100))
	// Simulate losing the acknowledgement after LOG_DB committed.
	require.NoError(t, model.DB.Model(&row).Update("effects_applied_at", 0).Error)
	require.NoError(t, RetryTaskSettlementEffects(context.Background(), 100))
	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("task_settlement_id = ?", row.ID).Count(&logs).Error)
	require.EqualValues(t, 1, logs)
	require.Equal(t, 1100, getUserQuota(t, 201))
}

func TestSettleTaskTerminalRejectsStaleLease(t *testing.T) {
	truncate(t)
	seedUser(t, 102, 1000)
	task := &model.Task{TaskID: "task-stale-lease", UserId: 102, Quota: 300, Status: model.TaskStatusInProgress, LastPollAt: 11, NextPollAt: 21, SettlementStatus: model.TaskSettlementStatusPending}
	require.NoError(t, model.DB.Create(task).Error)
	task.Status = model.TaskStatusSuccess
	task.NextPollAt = 22
	require.NoError(t, SettleTaskTerminal(context.Background(), task, model.TaskStatusInProgress, 200, 200, 0, "success"))
	var count int64
	require.NoError(t, model.DB.Model(&model.TaskSettlement{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}
