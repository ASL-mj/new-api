package model

import (
	"bytes"
	"database/sql"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyTaskSchema struct {
	ID     int64 `gorm:"primaryKey"`
	Status TaskStatus
}

func (legacyTaskSchema) TableName() string {
	return "tasks"
}

func prepareTaskSchemaMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	modelTestDBMutex.Lock()

	previousDB := DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		DB = previousDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		_ = sqlDB.Close()
		modelTestDBMutex.Unlock()
	})

	return db
}

func TestMigrateTaskSchemaPreservesLegacyTasksAndMarksTerminalTasks(t *testing.T) {
	db := prepareTaskSchemaMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&legacyTaskSchema{}))
	require.NoError(t, db.Create(&legacyTaskSchema{ID: 1, Status: TaskStatusSuccess}).Error)
	require.NoError(t, db.Create(&legacyTaskSchema{ID: 2, Status: TaskStatusSubmitted}).Error)
	require.NoError(t, db.Create(&legacyTaskSchema{ID: 3, Status: TaskStatusNotStart}).Error)

	require.NoError(t, migrateTaskSchema())

	for _, column := range []string{
		"poll_failures",
		"last_poll_at",
		"next_poll_at",
		"last_poll_error_class",
		"last_poll_error_message",
		"settlement_status",
		"settled_at",
	} {
		assert.Truef(t, db.Migrator().HasColumn(&Task{}, column), "missing column %s", column)
	}
	assert.True(t, db.Migrator().HasTable(&TaskSettlement{}))
	assert.True(t, db.Migrator().HasIndex(&Task{}, "idx_task_poll_ready"))
	assert.True(t, db.Migrator().HasIndex(&Task{}, "idx_task_settlement_migration_v1"))

	var completed Task
	require.NoError(t, db.First(&completed, 1).Error)
	assert.EqualValues(t, TaskStatusSuccess, completed.Status)
	assert.Equal(t, TaskSettlementStatusLegacy, completed.SettlementStatus)

	var inFlight Task
	require.NoError(t, db.First(&inFlight, 2).Error)
	assert.EqualValues(t, TaskStatusSubmitted, inFlight.Status)
	assert.Equal(t, TaskSettlementStatusPending, inFlight.SettlementStatus)

	var historicalNotStart Task
	require.NoError(t, db.First(&historicalNotStart, 3).Error)
	assert.EqualValues(t, TaskStatusSubmitted, historicalNotStart.Status)

	var settlementCount int64
	require.NoError(t, db.Model(&TaskSettlement{}).Count(&settlementCount).Error)
	assert.Zero(t, settlementCount)
}

func TestTaskSettlementAllowsOnlyOneTerminalBillingAnchorPerTask(t *testing.T) {
	truncateTables(t)

	task := &Task{TaskID: "task_settlement_unique", Status: TaskStatusSuccess}
	insertTask(t, task)

	first := &TaskSettlement{
		TaskID:           task.ID,
		Kind:             TaskSettlementKindSettle,
		State:            TaskSettlementStatePending,
		PreConsumedQuota: 100,
		QuotaDelta:       -10,
		ReasonClass:      "provider_terminal",
		PricingSnapshot:  []byte(`{"version":"s0"}`),
	}
	require.NoError(t, DB.Create(first).Error)

	duplicate := &TaskSettlement{
		TaskID: task.ID,
		Kind:   TaskSettlementKindRefund,
		State:  TaskSettlementStatePending,
	}
	assert.Error(t, DB.Create(duplicate).Error)
}

func TestTaskBindingRetainsRecoveryRecordUntilUpstreamIDIsStored(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID:           "task_submission_boundary",
		Status:           TaskStatusNotStart,
		SettlementStatus: TaskSettlementStatusPending,
		Quota:            80,
		PrivateData: TaskPrivateData{
			ChannelKeyFingerprint: "fingerprint",
			BillingContext: &TaskBillingContext{
				ModelPrice:      0.01,
				GroupRatio:      1,
				OriginModelName: "video-model",
			},
		},
	}
	insertTask(t, task)

	var pending Task
	require.NoError(t, DB.First(&pending, task.ID).Error)
	assert.Equal(t, TaskStatusNotStart, pending.Status)
	assert.Empty(t, pending.PrivateData.UpstreamTaskID)
	assert.Equal(t, "fingerprint", pending.PrivateData.ChannelKeyFingerprint)
	assert.NotNil(t, pending.PrivateData.BillingContext)

	task.SetData(map[string]string{"state": "accepted"})
	won, err := task.BindUpstreamSubmission("upstream-task-123", task.Data, 96)
	require.NoError(t, err)
	assert.True(t, won)

	var submitted Task
	require.NoError(t, DB.First(&submitted, task.ID).Error)
	assert.EqualValues(t, TaskStatusSubmitted, submitted.Status)
	assert.Equal(t, "upstream-task-123", submitted.PrivateData.UpstreamTaskID)
	assert.Equal(t, 96, submitted.Quota)
	assert.JSONEq(t, `{"state":"accepted"}`, string(submitted.Data))
}

func TestLegacyPollingExcludesUnboundSubmissionRecoveryRecords(t *testing.T) {
	truncateTables(t)

	cutoff := time.Now().Add(-time.Hour).Unix()
	insertTask(t, &Task{
		TaskID:     "task_unbound_recovery",
		Status:     TaskStatusNotStart,
		Progress:   "0%",
		SubmitTime: cutoff - 1,
	})
	active := &Task{
		TaskID:     "task_active_submission",
		Status:     TaskStatusSubmitted,
		Progress:   "0%",
		SubmitTime: cutoff - 1,
	}
	insertTask(t, active)

	activeTasks := GetAllUnFinishSyncTasks(10)
	require.Len(t, activeTasks, 1)
	assert.Equal(t, active.TaskID, activeTasks[0].TaskID)

	timedOutTasks := GetTimedOutUnfinishedTasks(cutoff, 10)
	require.Len(t, timedOutTasks, 1)
	assert.Equal(t, active.TaskID, timedOutTasks[0].TaskID)
}

func TestTaskSchemaMigrationDDLIsCrossDatabaseCompatible(t *testing.T) {
	db := prepareTaskSchemaMigrationDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	testCases := []struct {
		name string
		open func(*sql.DB, *bytes.Buffer) (*gorm.DB, error)
	}{
		{
			name: "sqlite",
			open: func(conn *sql.DB, output *bytes.Buffer) (*gorm.DB, error) {
				return gorm.Open(sqlite.Dialector{Conn: conn}, &gorm.Config{
					DryRun: true,
					Logger: logger.New(log.New(output, "", 0), logger.Config{LogLevel: logger.Info}),
				})
			},
		},
		{
			name: "mysql",
			open: func(conn *sql.DB, output *bytes.Buffer) (*gorm.DB, error) {
				return gorm.Open(mysql.New(mysql.Config{
					Conn:                      conn,
					SkipInitializeWithVersion: true,
				}), &gorm.Config{
					DryRun: true,
					Logger: logger.New(log.New(output, "", 0), logger.Config{LogLevel: logger.Info}),
				})
			},
		},
		{
			name: "postgres",
			open: func(conn *sql.DB, output *bytes.Buffer) (*gorm.DB, error) {
				return gorm.Open(postgres.New(postgres.Config{
					Conn:             conn,
					WithoutReturning: true,
				}), &gorm.Config{
					DryRun: true,
					Logger: logger.New(log.New(output, "", 0), logger.Config{LogLevel: logger.Info}),
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			dryRunDB, err := tc.open(sqlDB, &output)
			require.NoError(t, err)
			require.NoError(t, dryRunDB.Migrator().CreateTable(&Task{}, &TaskSettlement{}))

			ddl := strings.ToUpper(output.String())
			assert.Contains(t, ddl, "CREATE TABLE")
			assert.Contains(t, ddl, "TASK_SETTLEMENTS")
			assert.Contains(t, ddl, "IDX_TASK_POLL_READY")
			assert.NotContains(t, ddl, "JSONB")
		})
	}
}
