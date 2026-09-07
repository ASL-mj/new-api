package relay

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type submissionOutcomeUnknownFixture struct{}

func (*submissionOutcomeUnknownFixture) Error() string                  { return "fixture submission outcome unknown" }
func (*submissionOutcomeUnknownFixture) SubmissionOutcomeUnknown() bool { return true }

func prepareTaskSubmissionDB(t *testing.T) {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Task{}))

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		_ = sqlDB.Close()
	})
}

func TestPersistTaskBeforeUpstreamRetainsOneRecoveryRecordAcrossRetries(t *testing.T) {
	prepareTaskSubmissionDB(t)

	info := &relaycommon.RelayInfo{
		UserId:          12,
		UsingGroup:      "default",
		OriginModelName: "video-model",
		PriceData: types.PriceData{
			Quota:          50,
			ModelPrice:     0.01,
			ModelRatio:     1,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			OtherRatios:    map[string]float64{"seconds": 1},
		},
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: 101},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_submission_retry", Action: "video_generation"},
	}

	first, err := persistTaskBeforeUpstream(constant.TaskPlatform("hailuo"), info)
	require.NoError(t, err)
	assert.NotZero(t, first.ID)
	assert.Equal(t, first.ID, info.PersistedTaskID)
	assert.EqualValues(t, model.TaskStatusNotStart, first.Status)
	assert.Empty(t, first.PrivateData.UpstreamTaskID)
	assert.Equal(t, 101, first.ChannelId)
	assert.Equal(t, 50, first.Quota)

	info.ChannelId = 202
	info.PriceData.Quota = 80
	info.PriceData.OtherRatios["seconds"] = 2
	retry, err := persistTaskBeforeUpstream(constant.TaskPlatform("hailuo"), info)
	require.NoError(t, err)
	assert.Equal(t, first.ID, retry.ID)
	assert.EqualValues(t, model.TaskStatusNotStart, retry.Status)
	assert.Equal(t, 202, retry.ChannelId)
	assert.Equal(t, 80, retry.Quota)
	assert.Equal(t, 2.0, retry.PrivateData.BillingContext.OtherRatios["seconds"])

	var count int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestTaskSubmissionDoRequestErrorPreservesUnknownOutcome(t *testing.T) {
	prepareTaskSubmissionDB(t)
	task := &model.Task{
		TaskID:          "task_unknown_transport",
		Status:          model.TaskStatusNotStart,
		SubmissionState: service.TaskSubmissionPrepared,
	}
	require.NoError(t, model.DB.Create(task).Error)

	taskErr := taskSubmissionDoRequestError(context.Background(), task, &submissionOutcomeUnknownFixture{})
	require.NotNil(t, taskErr)
	assert.Equal(t, service.TaskSubmissionUnknownCode, taskErr.Code)
	assert.True(t, taskErr.LocalError)
	assert.True(t, service.TaskSubmissionRetainsFunding(taskErr.Code))

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, service.TaskSubmissionUnknown, stored.SubmissionState)
	assert.Equal(t, model.TaskStatus(model.TaskStatusNotStart), stored.Status)
	assert.Empty(t, stored.PrivateData.UpstreamTaskID)
}
