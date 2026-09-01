package controller

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareMonitorRunnerTables(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRecordSystemEvent := recordMonitorSystemEvent

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	recordMonitorSystemEvent = func(model.SystemEventLog) {}
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Log{}, &model.MonitorGroup{}, &model.MonitorGroupTarget{}, &model.MonitorCheck{}, &model.OpsMetricBucket{}, &model.OpsMetricHistogram{}))

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		recordMonitorSystemEvent = previousRecordSystemEvent
		_ = sqlDB.Close()
	})
}

func withMonitorProbe(t *testing.T, probe func(*model.Channel, string, time.Duration) channelProbeResult) {
	t.Helper()
	original := monitorProbeFunc
	monitorProbeFunc = probe
	t.Cleanup(func() {
		monitorProbeFunc = original
	})
}

func withMonitorEndpointPing(t *testing.T, ping func(*model.Channel, time.Duration) *int64) {
	t.Helper()
	original := monitorEndpointPingFunc
	monitorEndpointPingFunc = ping
	t.Cleanup(func() {
		monitorEndpointPingFunc = original
	})
}

func newMonitorRunnerState(t *testing.T, workerCount int) *monitorGroupRunnerState {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	state := &monitorGroupRunnerState{
		ctx:       ctx,
		jobs:      make(chan monitorProbeJob, 16),
		workerNum: workerCount,
	}
	for i := 0; i < workerCount; i++ {
		go monitorProbeWorker(state)
	}
	t.Cleanup(cancel)
	return state
}

func TestMonitorGroupRunnerRecordsOperationalDegradedAndTimeoutChecks(t *testing.T) {
	prepareMonitorRunnerTables(t)
	events := make([]model.SystemEventLog, 0, 2)
	recordMonitorSystemEvent = func(event model.SystemEventLog) {
		events = append(events, event)
	}

	channel := &model.Channel{Type: 1, Key: "test-key", Name: "test channel", Models: "gpt-5.4,gpt-5.4-mini"}
	require.NoError(t, model.DB.Create(channel).Error)
	group := &model.MonitorGroup{
		Name:            "Primary",
		Key:             "primary",
		PrimaryModel:    "gpt-5.4",
		ExtraModels:     `["gpt-5.4-mini","timeout-model"]`,
		Enabled:         true,
		UserVisible:     true,
		IntervalSeconds: 600,
		TimeoutSeconds:  5,
		DegradedMs:      500,
	}
	require.NoError(t, model.CreateMonitorGroup(group, []int{channel.Id}))

	withMonitorProbe(t, func(_ *model.Channel, modelName string, _ time.Duration) channelProbeResult {
		switch modelName {
		case "gpt-5.4":
			return channelProbeResult{Success: true, LatencyMs: 120}
		case "gpt-5.4-mini":
			return channelProbeResult{Success: true, LatencyMs: 900}
		default:
			return channelProbeResult{Success: false, LatencyMs: 5000, ErrorCode: "timeout", Message: "context deadline exceeded"}
		}
	})

	runMonitorGroup(newMonitorRunnerState(t, 2), group)

	var checks []model.MonitorCheck
	require.NoError(t, model.DB.Order("model_name ASC").Find(&checks).Error)
	require.Len(t, checks, 3)
	assert.Equal(t, model.MonitorCheckStatusOperational, checks[0].Status)
	assert.Equal(t, "gpt-5.4", checks[0].ModelName)
	assert.Equal(t, model.MonitorCheckStatusDegraded, checks[1].Status)
	require.Len(t, events, 2)
	assert.Equal(t, "system_event.monitor_probe_started", events[0].MessageKey)
	assert.Equal(t, "system_event.monitor_probe_completed", events[1].MessageKey)
	assert.Contains(t, events[1].Extra, `"operational":1`)
	assert.Equal(t, "gpt-5.4-mini", checks[1].ModelName)
	assert.Equal(t, model.MonitorCheckStatusTimeout, checks[2].Status)
	assert.Equal(t, "timeout", checks[2].ErrorCode)

	updated, err := model.GetMonitorGroupById(group.Id)
	require.NoError(t, err)
	assert.NotZero(t, updated.LastCheckedAt)
}

func TestMonitorGroupRunnerUsesTargetModelOverrideAndRecoversPanic(t *testing.T) {
	prepareMonitorRunnerTables(t)

	channel := &model.Channel{Type: 1, Key: "test-key", Name: "test channel"}
	require.NoError(t, model.DB.Create(channel).Error)
	group := &model.MonitorGroup{Name: "Override", Key: "override", PrimaryModel: "ignored", Enabled: true, UserVisible: true, TimeoutSeconds: 5, DegradedMs: 500}
	require.NoError(t, model.CreateMonitorGroup(group, []int{channel.Id}))
	require.NoError(t, model.DB.Model(&model.MonitorGroupTarget{}).Where("monitor_group_id = ?", group.Id).Update("models", `["fast","panic"]`).Error)

	withMonitorProbe(t, func(_ *model.Channel, modelName string, _ time.Duration) channelProbeResult {
		if modelName == "panic" {
			panic("upstream adapter panic")
		}
		return channelProbeResult{Success: true, LatencyMs: 10}
	})

	runMonitorGroup(newMonitorRunnerState(t, 1), group)

	var checks []model.MonitorCheck
	require.NoError(t, model.DB.Order("model_name ASC").Find(&checks).Error)
	require.Len(t, checks, 2)
	assert.Equal(t, "fast", checks[0].ModelName)
	assert.Equal(t, model.MonitorCheckStatusOperational, checks[0].Status)
	assert.Equal(t, "panic", checks[1].ModelName)
	assert.Equal(t, model.MonitorCheckStatusFailed, checks[1].Status)
	assert.Equal(t, "probe_panic", checks[1].ErrorCode)
}

func TestStartMonitorGroupRunnerStartsWorkersOnSlaveWithoutScheduler(t *testing.T) {
	prepareMonitorRunnerTables(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	StopMonitorGroupRunnerForTest()
	t.Cleanup(func() {
		StopMonitorGroupRunnerForTest()
		common.IsMasterNode = previousMaster
	})

	StartMonitorGroupRunner()

	monitorGroupRunner.Lock()
	state := monitorGroupRunner.state
	monitorGroupRunner.Unlock()
	require.NotNil(t, state)
	assert.False(t, state.schedulerStarted)
}

func TestStartMonitorGroupRunnerStartsSchedulerOnlyOnMaster(t *testing.T) {
	prepareMonitorRunnerTables(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = true
	StopMonitorGroupRunnerForTest()
	t.Cleanup(func() {
		StopMonitorGroupRunnerForTest()
		common.IsMasterNode = previousMaster
	})

	StartMonitorGroupRunner()

	monitorGroupRunner.Lock()
	state := monitorGroupRunner.state
	monitorGroupRunner.Unlock()
	require.NotNil(t, state)
	assert.True(t, state.schedulerStarted)
}

func TestRunMonitorGroupNowWorksOnSlave(t *testing.T) {
	prepareMonitorRunnerTables(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	StopMonitorGroupRunnerForTest()
	t.Cleanup(func() {
		StopMonitorGroupRunnerForTest()
		common.IsMasterNode = previousMaster
	})

	channel := &model.Channel{Type: 1, Key: "test-key", Name: "slave channel", Models: "gpt-5.4"}
	require.NoError(t, model.DB.Create(channel).Error)
	group := &model.MonitorGroup{Name: "Slave", Key: "slave", PrimaryModel: "gpt-5.4", Enabled: true, UserVisible: true, IntervalSeconds: 600, TimeoutSeconds: 5, DegradedMs: 500}
	require.NoError(t, model.CreateMonitorGroup(group, []int{channel.Id}))
	withMonitorProbe(t, func(_ *model.Channel, _ string, _ time.Duration) channelProbeResult {
		return channelProbeResult{Success: true, LatencyMs: 12}
	})
	withMonitorEndpointPing(t, func(_ *model.Channel, _ time.Duration) *int64 {
		value := int64(4)
		return &value
	})

	StartMonitorGroupRunner()
	require.NoError(t, RunMonitorGroupNow(group.Id))
	require.Eventually(t, func() bool {
		var count int64
		return model.DB.Model(&model.MonitorCheck{}).Where("monitor_group_id = ?", group.Id).Count(&count).Error == nil && count == 1
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return !isMonitorGroupRunning(group.Id)
	}, time.Second, 10*time.Millisecond)
}

func TestRunDueMonitorGroupsSchedulesWithoutManualTrigger(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Type: 1, Key: "test-key", Name: "scheduled", Models: "gpt-5.4"}
	require.NoError(t, model.DB.Create(channel).Error)
	group := &model.MonitorGroup{Name: "Scheduled", Key: "scheduled", PrimaryModel: "gpt-5.4", Enabled: true, UserVisible: true, IntervalSeconds: 15, TimeoutSeconds: 5, DegradedMs: 500}
	require.NoError(t, model.CreateMonitorGroup(group, []int{channel.Id}))
	withMonitorProbe(t, func(_ *model.Channel, _ string, _ time.Duration) channelProbeResult {
		return channelProbeResult{Success: true, LatencyMs: 12}
	})
	withMonitorEndpointPing(t, func(_ *model.Channel, _ time.Duration) *int64 { return nil })

	runDueMonitorGroups(newMonitorRunnerState(t, 1))
	require.Eventually(t, func() bool {
		var count int64
		return model.DB.Model(&model.MonitorCheck{}).Where("monitor_group_id = ?", group.Id).Count(&count).Error == nil && count == 1
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return !isMonitorGroupRunning(group.Id)
	}, time.Second, 10*time.Millisecond)
}

func TestRunDueMonitorGroupsReportsQueryFailureInHeartbeat(t *testing.T) {
	prepareMonitorRunnerTables(t)
	previousGetter := getEnabledMonitorGroups
	getEnabledMonitorGroups = func() ([]*model.MonitorGroup, error) {
		return nil, errors.New("database unavailable")
	}
	t.Cleanup(func() { getEnabledMonitorGroups = previousGetter })

	runDueMonitorGroups(newMonitorRunnerState(t, 1))

	var heartbeat *common.JobHeartbeat
	for _, item := range common.GetJobHeartbeats() {
		if item.Name == "monitor_group_runner" {
			copy := item
			heartbeat = &copy
			break
		}
	}
	require.NotNil(t, heartbeat)
	assert.Equal(t, "error", heartbeat.Status)
	assert.Equal(t, "failed to load enabled monitor groups", heartbeat.Message)
}

func TestMonitorGroupRunnerPingsEachChannelOncePerRun(t *testing.T) {
	prepareMonitorRunnerTables(t)

	channelA := &model.Channel{Type: 1, Key: "a", Name: "A", Models: "gpt-5.4,gpt-5.4-mini"}
	channelB := &model.Channel{Type: 1, Key: "b", Name: "B", Models: "gpt-5.4,gpt-5.4-mini"}
	require.NoError(t, model.DB.Create(channelA).Error)
	require.NoError(t, model.DB.Create(channelB).Error)
	group := &model.MonitorGroup{Name: "Ping", Key: "ping", PrimaryModel: "gpt-5.4", ExtraModels: `["gpt-5.4-mini"]`, Enabled: true, UserVisible: true, TimeoutSeconds: 5, DegradedMs: 500}
	require.NoError(t, model.CreateMonitorGroup(group, []int{channelA.Id, channelB.Id}))
	withMonitorProbe(t, func(_ *model.Channel, _ string, _ time.Duration) channelProbeResult {
		return channelProbeResult{Success: true, LatencyMs: 10}
	})
	var pingCalls atomic.Int64
	withMonitorEndpointPing(t, func(channel *model.Channel, _ time.Duration) *int64 {
		value := int64(channel.Id * 10)
		pingCalls.Add(1)
		return &value
	})

	runMonitorGroup(newMonitorRunnerState(t, 2), group)

	assert.EqualValues(t, 2, pingCalls.Load())
	var checks []model.MonitorCheck
	require.NoError(t, model.DB.Order("channel_id ASC, model_name ASC").Find(&checks).Error)
	require.Len(t, checks, 4)
	for _, check := range checks {
		require.NotNil(t, check.PingLatencyMs)
		assert.EqualValues(t, check.ChannelId*10, *check.PingLatencyMs)
	}
}

func TestMonitorGroupRunnerDoesNotFailProbeWhenPingFails(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Type: 1, Key: "test-key", Name: "No Ping", Models: "gpt-5.4"}
	require.NoError(t, model.DB.Create(channel).Error)
	group := &model.MonitorGroup{Name: "No Ping", Key: "no-ping", PrimaryModel: "gpt-5.4", Enabled: true, UserVisible: true, TimeoutSeconds: 5, DegradedMs: 500}
	require.NoError(t, model.CreateMonitorGroup(group, []int{channel.Id}))
	withMonitorProbe(t, func(_ *model.Channel, _ string, _ time.Duration) channelProbeResult {
		return channelProbeResult{Success: true, LatencyMs: 10}
	})
	withMonitorEndpointPing(t, func(_ *model.Channel, _ time.Duration) *int64 { return nil })

	runMonitorGroup(newMonitorRunnerState(t, 1), group)

	var check model.MonitorCheck
	require.NoError(t, model.DB.First(&check).Error)
	assert.Equal(t, model.MonitorCheckStatusOperational, check.Status)
	assert.Nil(t, check.PingLatencyMs)
}
