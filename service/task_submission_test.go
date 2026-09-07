package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmissionRecoveryIsIdempotent(t *testing.T) {
	truncate(t)
	seedUser(t, 301, 1000)
	seedToken(t, 301, 301, "submission-recovery-token", 1000)
	seedChannelWithQuotaLimit(t, 301, 10000)

	task := &model.Task{
		TaskID:         "task_submission_recovery",
		UserId:         301,
		ChannelId:      301,
		Quota:          300,
		Group:          "default",
		Status:         model.TaskStatusNotStart,
		PollingVersion: 2,
		Properties:     model.Properties{OriginModelName: "test-model"},
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceWallet,
			TokenId:       301,
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "test-model",
				GroupRatio:      1,
			},
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	info := &relaycommon.RelayInfo{
		UserId:          301,
		UsingGroup:      "default",
		OriginModelName: "test-model",
		PriceData: types.PriceData{
			Quota:          300,
			ModelRatio:     1,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: task.TaskID, TaskPollingVersion: 2},
	}

	require.NoError(t, PrepareTaskSubmissionAccounting(context.Background(), task, info))
	require.NoError(t, stageTaskSubmission(context.Background(), task.ID, "upstream_recovery_fixture", []byte("{\"task_id\":\"upstream_recovery_fixture\"}"), info))

	var staged model.Task
	require.NoError(t, model.DB.First(&staged, task.ID).Error)
	require.Equal(t, TaskSubmissionAccepted, staged.SubmissionState)
	require.False(t, staged.SubmissionSettled)
	require.Equal(t, "upstream_recovery_fixture", staged.PrivateData.UpstreamTaskID)

	require.NoError(t, RetryTaskSubmissionAccounting(context.Background(), 100))
	require.NoError(t, RetryTaskSubmissionAccounting(context.Background(), 100))

	var recovered model.Task
	require.NoError(t, model.DB.First(&recovered, task.ID).Error)
	require.Equal(t, TaskSubmissionApplied, recovered.SubmissionState)
	require.True(t, recovered.SubmissionSettled)
	require.Equal(t, model.TaskStatus(model.TaskStatusSubmitted), recovered.Status)
	require.Equal(t, 300, recovered.Quota)

	var user model.User
	require.NoError(t, model.DB.First(&user, 301).Error)
	require.Equal(t, 300, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	channel := getChannelQuotaState(t, 301)
	require.EqualValues(t, 300, channel.QuotaLimitUsed)

	require.NoError(t, RetryTaskSubmissionEffects(context.Background(), 100))
	require.NoError(t, model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("submission_effects_at", 0).Error)
	require.NoError(t, RetryTaskSubmissionEffects(context.Background(), 100))

	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("task_submission_id = ?", task.ID).Count(&logs).Error)
	require.EqualValues(t, 1, logs)
	require.NoError(t, model.DB.First(&user, 301).Error)
	require.Equal(t, 300, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	channel = getChannelQuotaState(t, 301)
	require.EqualValues(t, 300, channel.QuotaLimitUsed)
}
