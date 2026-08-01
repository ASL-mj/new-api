package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorChecksLatestTimelineAndAvailability(t *testing.T) {
	prepareMonitorTables(t)
	now := time.Now().Unix()

	rows := []*MonitorCheck{
		{MonitorGroupId: 1, ChannelId: 1, ModelName: "gpt-5.4", Status: MonitorCheckStatusOperational, LatencyMs: 120, CheckedAt: now - 120},
		{MonitorGroupId: 1, ChannelId: 1, ModelName: "gpt-5.4", Status: MonitorCheckStatusFailed, ErrorCode: "upstream_500", CheckedAt: now - 30},
		{MonitorGroupId: 1, ChannelId: 1, ModelName: "gpt-5.4", Status: MonitorCheckStatusTimeout, ErrorCode: "timeout", CheckedAt: now - 30},
		{MonitorGroupId: 1, ChannelId: 1, ModelName: "gpt-5.4-mini", Status: MonitorCheckStatusOperational, CheckedAt: now - 20},
		{MonitorGroupId: 1, ChannelId: 2, ModelName: "gpt-5.4", Status: MonitorCheckStatusDegraded, CheckedAt: now - 10},
		{MonitorGroupId: 2, ChannelId: 3, ModelName: "gpt-5.4", Status: MonitorCheckStatusTimeout, CheckedAt: now - 5},
	}
	require.NoError(t, InsertMonitorChecks(rows))

	latest, err := GetLatestMonitorChecks([]int{1})
	require.NoError(t, err)
	require.Len(t, latest, 3)
	assert.Equal(t, MonitorCheckStatusTimeout, latest[0].Status)
	assert.Equal(t, "gpt-5.4", latest[0].ModelName)
	assert.Equal(t, "gpt-5.4-mini", latest[1].ModelName)
	assert.Equal(t, MonitorCheckStatusDegraded, latest[2].Status)

	timeline, err := GetMonitorTimeline(1, "gpt-5.4", 2)
	require.NoError(t, err)
	require.Len(t, timeline, 2)
	assert.Equal(t, MonitorCheckStatusTimeout, timeline[0].Status)
	assert.Equal(t, MonitorCheckStatusDegraded, timeline[1].Status)

	availability, err := GetMonitorAvailability(1, 1)
	require.NoError(t, err)
	require.Len(t, availability, 1)
	assert.EqualValues(t, 5, availability[0].CheckCount)
	assert.EqualValues(t, 3, availability[0].AvailableCount)
	assert.Equal(t, 60.0, availability[0].AvailabilityPct)

	allLatest, err := GetLatestMonitorChecks(nil)
	require.NoError(t, err)
	assert.Len(t, allLatest, 4)
	emptyLatest, err := GetLatestMonitorChecks([]int{})
	require.NoError(t, err)
	assert.Empty(t, emptyLatest)
}

func TestMonitorCheckDeleteExpiredKeepsBoundary(t *testing.T) {
	prepareMonitorTables(t)

	require.NoError(t, InsertMonitorChecks([]*MonitorCheck{
		{MonitorGroupId: 1, ChannelId: 1, ModelName: "gpt-5.4", Status: MonitorCheckStatusOperational, CheckedAt: 100},
		{MonitorGroupId: 1, ChannelId: 1, ModelName: "gpt-5.4", Status: MonitorCheckStatusOperational, CheckedAt: 200},
		{MonitorGroupId: 1, ChannelId: 1, ModelName: "gpt-5.4", Status: MonitorCheckStatusOperational, CheckedAt: 300},
	}))

	deleted, err := DeleteExpiredMonitorChecks(200)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)

	var remaining []MonitorCheck
	require.NoError(t, DB.Order("checked_at ASC").Find(&remaining).Error)
	require.Len(t, remaining, 2)
	assert.EqualValues(t, 200, remaining[0].CheckedAt)
	assert.EqualValues(t, 300, remaining[1].CheckedAt)
}

func TestMonitorCheckStoresNullablePingLatency(t *testing.T) {
	prepareMonitorTables(t)
	ping := int64(42)
	require.NoError(t, InsertMonitorChecks([]*MonitorCheck{
		{MonitorGroupId: 1, ChannelId: 1, ModelName: "with-ping", Status: MonitorCheckStatusOperational, PingLatencyMs: &ping, CheckedAt: 100},
		{MonitorGroupId: 1, ChannelId: 2, ModelName: "without-ping", Status: MonitorCheckStatusOperational, CheckedAt: 101},
	}))

	var rows []MonitorCheck
	require.NoError(t, DB.Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.NotNil(t, rows[0].PingLatencyMs)
	assert.EqualValues(t, 42, *rows[0].PingLatencyMs)
	assert.Nil(t, rows[1].PingLatencyMs)
}
