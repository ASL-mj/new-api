package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateSunoTaskFromResponseRefundsOnlyCASWinner(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 23, 23, 123
	const preConsumed = 40
	when := time.Date(2026, 8, 2, 16, 0, 0, 0, time.UTC)

	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-suno-cas", 1000)
	channel := &model.Channel{
		Id:             channelID,
		Name:           "suno-cas-channel",
		Key:            "sk-suno-cas",
		Status:         common.ChannelStatusEnabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeBoth,
		QuotaLimit:     200,
		Group:          "default",
		Models:         "suno-v3",
	}
	seedTaskBillingUsageChannel(t, channel)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.TaskID = "suno-cas-task"
	task.PrivateData.ChannelKeyFingerprint = applyTaskPreconsume(t, when, task, channel.Key, 0)
	task.PrivateData.ChannelKeyIndex = 0
	require.NoError(t, model.DB.Create(task).Error)

	var firstPoll model.Task
	var concurrentPoll model.Task
	require.NoError(t, model.DB.First(&firstPoll, task.ID).Error)
	require.NoError(t, model.DB.First(&concurrentPoll, task.ID).Error)

	response := dto.SunoDataResponse{
		TaskID:     task.TaskID,
		Status:     model.TaskStatusFailure,
		FailReason: "upstream failed",
		FinishTime: when.Add(time.Minute).Unix(),
	}
	require.NoError(t, updateSunoTaskFromResponse(ctx, &firstPoll, response))
	require.NoError(t, updateSunoTaskFromResponse(ctx, &concurrentPoll, response))

	assert.Equal(t, 10000+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, 1000+preConsumed, getTokenRemainQuota(t, tokenID))

	reloaded := getChannelQuotaState(t, channelID)
	assert.EqualValues(t, 0, reloaded.UsedQuota)
	assert.EqualValues(t, 0, reloaded.QuotaLimitUsed)
	keyUsage := getChannelKeyUsageByFingerprint(t, channelID, task.PrivateData.ChannelKeyFingerprint)
	assert.EqualValues(t, 0, keyUsage.QuotaLimitUsed)

	var refundLogs int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&refundLogs).Error)
	assert.EqualValues(t, 1, refundLogs)
}
