package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareMonitorTables(t *testing.T) {
	t.Helper()

	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	isolateDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := isolateDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	DB = isolateDB
	LOG_DB = isolateDB
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, isolateDB.AutoMigrate(&MonitorGroup{}, &MonitorGroupTarget{}, &MonitorCheck{}))

	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		_ = sqlDB.Close()
	})
}

func TestMonitorGroupCreateAndUpdateReplacesTargets(t *testing.T) {
	prepareMonitorTables(t)

	group := &MonitorGroup{
		Name:            "Core upstreams",
		Key:             "core-upstreams",
		PrimaryModel:    "gpt-5.4",
		Enabled:         true,
		UserVisible:     true,
		IntervalSeconds: 600,
		TimeoutSeconds:  30,
		DegradedMs:      3000,
	}
	require.NoError(t, CreateMonitorGroup(group, []int{8, 2, 8}))
	require.NotZero(t, group.Id)
	require.NotZero(t, group.CreatedAt)

	targets, err := GetMonitorGroupTargets(group.Id)
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, []int{8, 2}, []int{targets[0].ChannelId, targets[1].ChannelId})

	group.Name = "Core upstreams revised"
	group.LastCheckedAt = 999 // Request data must not overwrite scheduler state.
	require.NoError(t, DB.Model(&MonitorGroup{}).Where("id = ?", group.Id).Update("last_checked_at", int64(777)).Error)
	require.NoError(t, DB.Model(&MonitorGroup{}).Where("id = ?", group.Id).Updates(map[string]interface{}{
		"run_lease_until": int64(888), "run_lease_token": "active-lease",
	}).Error)
	require.NoError(t, UpdateMonitorGroup(group, []int{2, 5}))

	updated, err := GetMonitorGroupById(group.Id)
	require.NoError(t, err)
	assert.Equal(t, "core-upstreams", updated.Key)
	assert.Equal(t, "Core upstreams revised", updated.Name)
	assert.EqualValues(t, 777, updated.LastCheckedAt)
	assert.EqualValues(t, 888, updated.RunLeaseUntil)
	assert.Equal(t, "active-lease", updated.RunLeaseToken)

	targets, err = GetMonitorGroupTargets(group.Id)
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, []int{2, 5}, []int{targets[0].ChannelId, targets[1].ChannelId})
}

func TestMonitorGroupRunLeaseIsExclusiveAndTokenGuarded(t *testing.T) {
	prepareMonitorTables(t)
	group := &MonitorGroup{Name: "Lease", Key: "lease", PrimaryModel: "gpt-5.4", Enabled: true}
	require.NoError(t, CreateMonitorGroup(group, []int{1}))

	acquired, err := TryAcquireMonitorGroupRunLease(group.Id, "runner-a", 100, 200)
	require.NoError(t, err)
	assert.True(t, acquired)

	acquired, err = TryAcquireMonitorGroupRunLease(group.Id, "runner-b", 150, 250)
	require.NoError(t, err)
	assert.False(t, acquired)

	require.NoError(t, ReleaseMonitorGroupRunLease(group.Id, "runner-b"))
	persisted, err := GetMonitorGroupById(group.Id)
	require.NoError(t, err)
	assert.Equal(t, "runner-a", persisted.RunLeaseToken)

	acquired, err = TryAcquireMonitorGroupRunLease(group.Id, "runner-b", 200, 300)
	require.NoError(t, err)
	assert.True(t, acquired)

	// An older runner must not release a lease that has already been reclaimed.
	require.NoError(t, ReleaseMonitorGroupRunLease(group.Id, "runner-a"))
	persisted, err = GetMonitorGroupById(group.Id)
	require.NoError(t, err)
	assert.Equal(t, "runner-b", persisted.RunLeaseToken)
	require.NoError(t, ReleaseMonitorGroupRunLease(group.Id, "runner-b"))
}

func TestMonitorGroupUpdateRejectsKeyChangeWithoutReplacingTargets(t *testing.T) {
	prepareMonitorTables(t)

	group := &MonitorGroup{Name: "Stable", Key: "stable", PrimaryModel: "gpt-5.4"}
	require.NoError(t, CreateMonitorGroup(group, []int{1, 2}))

	group.Key = "changed"
	err := UpdateMonitorGroup(group, []int{3})
	require.EqualError(t, err, "monitor group key cannot be changed")

	targets, err := GetMonitorGroupTargets(group.Id)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, []int{targets[0].ChannelId, targets[1].ChannelId})
}

func TestMonitorGroupListFiltersAndDeletePreservesChecks(t *testing.T) {
	prepareMonitorTables(t)

	enabled := &MonitorGroup{Name: "Public Core", Key: "public-core", PrimaryModel: "gpt-5.4", Enabled: true}
	disabled := &MonitorGroup{Name: "Private Backup", Key: "private-backup", PrimaryModel: "gpt-5.4", Enabled: false}
	require.NoError(t, CreateMonitorGroup(enabled, []int{1}))
	require.NoError(t, CreateMonitorGroup(disabled, []int{2}))
	require.NoError(t, InsertMonitorChecks([]*MonitorCheck{{
		MonitorGroupId: enabled.Id,
		ChannelId:      1,
		ModelName:      "gpt-5.4",
		Status:         MonitorCheckStatusOperational,
		CheckedAt:      1,
	}}))

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	groups, total, err := GetMonitorGroups(pageInfo, "Public", 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, groups, 1)
	assert.Equal(t, enabled.Id, groups[0].Id)

	active, err := GetEnabledMonitorGroups()
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, enabled.Id, active[0].Id)

	require.NoError(t, DeleteMonitorGroup(enabled.Id))
	targets, err := GetMonitorGroupTargets(enabled.Id)
	require.NoError(t, err)
	assert.Empty(t, targets)

	var checkCount int64
	require.NoError(t, DB.Model(&MonitorCheck{}).Where("monitor_group_id = ?", enabled.Id).Count(&checkCount).Error)
	assert.EqualValues(t, 1, checkCount)
}
