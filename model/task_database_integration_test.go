package model

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Optional real-engine tests. DSNs must address loopback maintenance connections:
// TASK_TEST_MYSQL_DSN='root@tcp(127.0.0.1:3307)/'
// TASK_TEST_POSTGRES_DSN='host=127.0.0.1 port=55439 user=task_test dbname=postgres sslmode=disable'
// Each scenario creates and drops its own newapi_s1s2_test_* database. Existing
// databases are never reused; ordinary SQL_DSN / LOG_SQL_DSN are never consulted.
func TestTaskDatabaseIntegration(t *testing.T) {
	for _, engine := range []string{"mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			dsn := os.Getenv("TASK_TEST_" + strings.ToUpper(engine) + "_DSN")
			if dsn == "" {
				t.Skip("optional local database DSN is unset")
			}
			for _, scenario := range []struct {
				name string
				run  func(*testing.T, *gorm.DB)
			}{
				{"fresh", taskIntegrationFresh},
				{"legacy_upgrade", taskIntegrationLegacyUpgrade},
				{"interrupted_upgrade", taskIntegrationInterruptedUpgrade},
				{"constraints_and_polling", taskIntegrationConstraintsAndPolling},
				{"channel_accounting_rollback", taskIntegrationChannelAccounting},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					scenario.run(t, taskIntegrationDatabase(t, engine, dsn))
				})
			}
		})
	}
}

func taskIntegrationConnector(t *testing.T, engine, dsn string) func(string) (*sql.DB, error) {
	t.Helper()
	localHost := func(host string) string {
		if host == "localhost" {
			return "127.0.0.1"
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return host
		}
		t.Fatal("test connections require an explicit loopback host")
		return ""
	}
	if engine == "mysql" {
		cfg, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			t.Fatal("invalid MySQL test DSN (details suppressed to protect credentials)")
		}
		require.Empty(t, cfg.DBName, "MySQL test DSN must omit the database name")
		require.Equal(t, "tcp", cfg.Net, "MySQL test DSN must use TCP")
		host, port, err := net.SplitHostPort(cfg.Addr)
		if err != nil {
			t.Fatal("MySQL test DSN requires an explicit host and port")
		}
		cfg.Addr = net.JoinHostPort(localHost(host), port)
		cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 30*time.Second, 30*time.Second
		cfg.ParseTime = true
		return func(name string) (*sql.DB, error) {
			copy := cfg.Clone()
			copy.DBName = name
			return sql.Open("mysql", copy.FormatDSN())
		}
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid PostgreSQL test DSN (details suppressed to protect credentials)")
	}
	require.Equal(t, "postgres", cfg.Database, "PostgreSQL test DSN must use the maintenance database")
	cfg.Host = localHost(cfg.Host)
	// Eliminate DSN/env fallback hosts and session options before any connection.
	cfg.Fallbacks = nil
	cfg.RuntimeParams = map[string]string{"application_name": "newapi_s1s2_test"}
	cfg.ConnectTimeout = 5 * time.Second
	return func(name string) (*sql.DB, error) {
		copy := cfg.Copy()
		copy.Database = name
		return stdlib.OpenDB(*copy), nil
	}
}

func taskIntegrationDatabase(t *testing.T, engine, dsn string) *gorm.DB {
	t.Helper()
	open := taskIntegrationConnector(t, engine, dsn)
	maintenance, quote := "", "`"
	if engine == "postgres" {
		maintenance, quote = "postgres", `"`
	}
	admin, err := open(maintenance)
	if err != nil {
		t.Fatal("opening local maintenance connection failed (DSN suppressed)")
	}
	t.Cleanup(func() { assert.NoError(t, admin.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("local %s connection failed (%T; details suppressed)", engine, err)
	}
	name := fmt.Sprintf("newapi_s1s2_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	identifier := quote + name + quote
	createSQL := "CREATE DATABASE " + identifier
	if engine == "postgres" {
		createSQL += " TEMPLATE template0"
	}
	_, err = admin.ExecContext(ctx, createSQL)
	require.NoError(t, err, "create a new empty test database")
	t.Logf("created database %s", name)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.ExecContext(ctx, "DROP DATABASE "+identifier)
		if assert.NoError(t, err, "drop isolated test database %s", name) {
			t.Logf("dropped database %s", name)
		}
	})
	conn, err := open(name)
	if err != nil {
		t.Fatal("opening isolated test database failed (DSN suppressed)")
	}
	t.Cleanup(func() { assert.NoError(t, conn.Close()) })
	conn.SetMaxOpenConns(4)
	conn.SetMaxIdleConns(4)
	var dialector gorm.Dialector = mysql.New(mysql.Config{Conn: conn})
	identitySQL := "SELECT DATABASE()"
	if engine == "postgres" {
		dialector = postgres.New(postgres.Config{Conn: conn})
		identitySQL = "SELECT current_database()"
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("opening isolated %s GORM connection failed (%T; details suppressed)", engine, err)
	}
	var current, version string
	require.NoError(t, db.Raw(identitySQL).Scan(&current).Error)
	require.Equal(t, name, current)
	require.NoError(t, db.Raw("SELECT VERSION()").Scan(&version).Error)
	t.Logf("engine=%s version=%s database=%s", engine, version, current)
	tables, err := db.Migrator().GetTables()
	require.NoError(t, err)
	require.Empty(t, tables, "test database must start empty")

	modelTestDBMutex.Lock()
	previousDB, previousLogDB := DB, LOG_DB
	previousSQLite, previousMySQL, previousPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	previousColumns := []string{commonGroupCol, commonKeyCol, commonTrueVal, commonFalseVal, logGroupCol, logKeyCol}
	DB, LOG_DB = db, db
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = false, engine == "mysql", engine == "postgres"
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = previousSQLite, previousMySQL, previousPostgres
		commonGroupCol, commonKeyCol, commonTrueVal, commonFalseVal, logGroupCol, logKeyCol = previousColumns[0], previousColumns[1], previousColumns[2], previousColumns[3], previousColumns[4], previousColumns[5]
		modelTestDBMutex.Unlock()
	})
	return db
}

// Freeze the pre-S0 task columns and tags, rather than deriving the fixture from
// the current Task type: otherwise newly introduced columns could mask upgrades.
type taskIntegrationLegacyTask struct {
	ID          int64 `gorm:"primary_key;AUTO_INCREMENT"`
	CreatedAt   int64 `gorm:"index"`
	UpdatedAt   int64
	TaskID      string                `gorm:"type:varchar(191);index"`
	Platform    constant.TaskPlatform `gorm:"type:varchar(30);index"`
	UserId      int                   `gorm:"index"`
	Group       string                `gorm:"type:varchar(50)"`
	ChannelId   int                   `gorm:"index"`
	Quota       int
	Action      string     `gorm:"type:varchar(40);index"`
	Status      TaskStatus `gorm:"type:varchar(20);index"`
	FailReason  string
	SubmitTime  int64           `gorm:"index"`
	StartTime   int64           `gorm:"index"`
	FinishTime  int64           `gorm:"index"`
	Progress    string          `gorm:"type:varchar(20);index"`
	Properties  Properties      `gorm:"type:json"`
	PrivateData TaskPrivateData `gorm:"column:private_data;type:json"`
	Data        json.RawMessage `gorm:"type:json"`
}

func (taskIntegrationLegacyTask) TableName() string { return "tasks" }

func taskIntegrationFresh(t *testing.T, db *gorm.DB) {
	require.NoError(t, migrateTaskSchema())
	require.NoError(t, migrateLOGDB())
	taskIntegrationSchema(t, db)
	// A map deliberately omits fields so this tests server defaults, not GORM's
	// Go-side default population.
	require.NoError(t, db.Model(&Task{}).Create(map[string]any{"task_id": "default_probe", "status": TaskStatusSubmitted}).Error)
	var task Task
	require.NoError(t, db.Where("task_id = ?", "default_probe").First(&task).Error)
	assert.Zero(t, task.PollFailures)
	assert.Zero(t, task.LastPollAt)
	assert.Zero(t, task.NextPollAt)
	assert.Empty(t, task.LastPollErrorClass)
	assert.Zero(t, task.SettledAt)
	assert.Equal(t, TaskSettlementStatusPending, task.SettlementStatus)
	require.NoError(t, migrateTaskSchema(), "fresh migration is repeatable")
	require.NoError(t, migrateLOGDB(), "log migration is repeatable")
}

func taskIntegrationSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	columns, err := db.Migrator().ColumnTypes(&Task{})
	require.NoError(t, err)
	byName := make(map[string]gorm.ColumnType, len(columns))
	for _, column := range columns {
		byName[column.Name()] = column
	}
	for _, name := range []string{"poll_failures", "last_poll_at", "next_poll_at", "last_poll_error_class", "settlement_status", "settled_at"} {
		column, ok := byName[name]
		require.True(t, ok, "missing column %s", name)
		nullable, known := column.Nullable()
		assert.True(t, known, "missing nullability metadata for %s", name)
		assert.False(t, nullable, "%s must be NOT NULL", name)
		_, hasDefault := column.DefaultValue()
		assert.True(t, hasDefault, "%s must have a database default", name)
	}
	message, ok := byName["last_poll_error_message"]
	require.True(t, ok)
	nullable, known := message.Nullable()
	assert.True(t, known)
	assert.True(t, nullable)
	assert.True(t, db.Migrator().HasIndex(&Task{}, "idx_tasks_status"))
	assert.True(t, db.Migrator().HasIndex(&Task{}, "idx_task_settlement_migration_v1"))
	indexes, err := db.Migrator().GetIndexes(&Task{})
	require.NoError(t, err)
	found := false
	for _, index := range indexes {
		if index.Name() == "idx_task_poll_ready" {
			found = true
			orderedColumns := index.Columns()
			if db.Dialector.Name() == "postgres" {
				// postgres driver v1.5.2 GetIndexes uses ANY(indkey), losing order.
				orderedColumns = nil
				require.NoError(t, db.Raw("SELECT pg_get_indexdef(CAST(? AS regclass), n, true) FROM generate_series(1, 2) AS n ORDER BY n", "idx_task_poll_ready").Scan(&orderedColumns).Error)
			}
			assert.Equal(t, []string{"status", "next_poll_at"}, orderedColumns)
		}
	}
	assert.True(t, found, "missing ordered composite polling index")
	assert.True(t, db.Migrator().HasIndex(&TaskSettlement{}, "uq_task_settlements_task_id"))
	assert.True(t, db.Migrator().HasIndex(&Log{}, "uq_logs_task_settlement_id"))
}

func taskIntegrationSeedLegacy(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&taskIntegrationLegacyTask{}))
	for _, status := range []TaskStatus{TaskStatusSuccess, TaskStatusFailure, TaskStatusSubmitted, TaskStatusQueued, TaskStatusInProgress, TaskStatusNotStart} {
		row := taskIntegrationLegacyTask{
			TaskID: "legacy_" + string(status), Status: status, Quota: 100,
			Progress: "30%", Group: "fixture", SubmitTime: 100,
			Properties:  Properties{OriginModelName: "fixture-video"},
			PrivateData: TaskPrivateData{UpstreamTaskID: "fixture-upstream", ChannelStandardQuota: 50},
			Data:        json.RawMessage(`{"fixture":true}`),
		}
		require.NoError(t, db.Create(&row).Error)
	}
}

func taskIntegrationLegacyUpgrade(t *testing.T, db *gorm.DB) {
	taskIntegrationSeedLegacy(t, db)
	// Also cover adding the nullable log key to pre-existing ordinary log rows.
	legacyLog := struct {
		Id      int `gorm:"primaryKey"`
		Content string
	}{}
	require.NoError(t, db.Table("logs").AutoMigrate(&legacyLog))
	require.NoError(t, db.Table("logs").Create([]map[string]any{{"content": "old-a"}, {"content": "old-b"}}).Error)
	var ddl bytes.Buffer
	DB = db.Session(&gorm.Session{Logger: logger.New(log.New(&ddl, "", 0), logger.Config{LogLevel: logger.Info})})
	err := migrateTaskSchema()
	DB = db
	require.NoError(t, err)
	upper := strings.ToUpper(ddl.String())
	assert.NotContains(t, upper, "MODIFY COLUMN", "task upgrade must remain additive")
	assert.NotContains(t, upper, "ALTER COLUMN", "task upgrade must remain additive")
	assert.NotContains(t, upper, "DROP COLUMN")
	require.NoError(t, migrateLOGDB())
	taskIntegrationSchema(t, db)
	var rows []Task
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 6)
	for _, row := range rows {
		terminal := row.Status == TaskStatusSuccess || row.Status == TaskStatusFailure
		if terminal {
			assert.Equal(t, TaskSettlementStatusLegacy, row.SettlementStatus)
		} else {
			assert.Equal(t, TaskSettlementStatusPending, row.SettlementStatus)
		}
		assert.Zero(t, row.PollFailures)
		assert.Zero(t, row.NextPollAt)
		assert.Equal(t, 100, row.Quota)
		assert.Equal(t, "fixture", row.Group)
		assert.Equal(t, "fixture-video", row.Properties.OriginModelName)
		assert.Equal(t, "fixture-upstream", row.PrivateData.UpstreamTaskID)
		assert.JSONEq(t, `{"fixture":true}`, string(row.Data))
	}
	assert.Len(t, GetAllUnFinishSyncTasks(20), 4, "legacy unfinished work remains pollable")
	var count int64
	require.NoError(t, db.Model(&TaskSettlement{}).Count(&count).Error)
	assert.Zero(t, count, "migration must not replay legacy accounting")
	require.NoError(t, db.Model(&Log{}).Where("task_settlement_id IS NULL").Count(&count).Error)
	assert.EqualValues(t, 2, count)
	newTerminal := Task{TaskID: "new_terminal_pending", Status: TaskStatusSuccess}
	require.NoError(t, db.Create(&newTerminal).Error)
	require.NoError(t, migrateTaskSchema())
	require.NoError(t, db.First(&newTerminal, newTerminal.ID).Error)
	assert.Equal(t, TaskSettlementStatusPending, newTerminal.SettlementStatus, "repeat migration must not reclassify new terminal work")
}

func taskIntegrationInterruptedUpgrade(t *testing.T, db *gorm.DB) {
	taskIntegrationSeedLegacy(t, db)
	// MySQL DDL commits independently; a stop after adding settlement_status but
	// before the legacy data update must not make the next startup skip that update.
	require.NoError(t, db.Migrator().AddColumn(&Task{}, "SettlementStatus"))
	require.NoError(t, migrateTaskSchema())
	var terminal []Task
	require.NoError(t, db.Where("status IN ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).Find(&terminal).Error)
	require.Len(t, terminal, 2)
	for _, task := range terminal {
		assert.Equal(t, TaskSettlementStatusLegacy, task.SettlementStatus, "interrupted schema upgrade must still protect legacy terminal task %s", task.TaskID)
	}
}

func taskIntegrationConstraintsAndPolling(t *testing.T, db *gorm.DB) {
	require.NoError(t, migrateTaskSchema())
	require.NoError(t, migrateLOGDB())
	actual := 0
	settlement := TaskSettlement{TaskID: 1, Kind: TaskSettlementKindSettle, ActualQuota: &actual, PricingSnapshot: json.RawMessage(`{"version":"fixture"}`)}
	require.NoError(t, db.Create(&settlement).Error)
	var stored TaskSettlement
	require.NoError(t, db.First(&stored, settlement.ID).Error)
	require.NotNil(t, stored.ActualQuota)
	assert.Zero(t, *stored.ActualQuota)
	assert.JSONEq(t, `{"version":"fixture"}`, string(stored.PricingSnapshot))
	assert.Equal(t, TaskSettlementStatePending, stored.State)
	assert.Zero(t, stored.AttemptCount)
	assert.Zero(t, stored.AppliedAt)
	assert.Zero(t, stored.EffectsAppliedAt)
	assert.Zero(t, stored.EffectsNextAt)
	assert.Zero(t, stored.EffectsAttempts)
	duplicate := TaskSettlement{TaskID: 1, Kind: TaskSettlementKindRefund}
	assert.Error(t, db.Create(&duplicate).Error, "different kinds still share one unique task anchor")
	refund := TaskSettlement{TaskID: 2, Kind: TaskSettlementKindRefund, QuotaDelta: -100}
	require.NoError(t, db.Create(&refund).Error)
	require.NoError(t, db.First(&refund, refund.ID).Error)
	assert.Nil(t, refund.ActualQuota)
	assert.Nil(t, refund.PricingSnapshot)
	assert.Error(t, db.Model(&TaskSettlement{}).Create(map[string]any{"task_id": nil, "kind": "refund"}).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&Log{Content: "ordinary log"}).Error)
	}
	require.NoError(t, db.Create(&Log{TaskSettlementID: &settlement.ID, Quota: 50}).Error)
	assert.Error(t, db.Create(&Log{TaskSettlementID: &settlement.ID, Quota: 999}).Error)
	require.NoError(t, db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Log{TaskSettlementID: &settlement.ID, Quota: 999}).Error)
	var logs []Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 4, "replayed log must not duplicate the committed fact")
	for _, row := range logs {
		if row.TaskSettlementID != nil {
			assert.Equal(t, 50, row.Quota)
		}
	}
	tasks := []Task{
		{TaskID: "ready", Status: TaskStatusSubmitted, PollingVersion: 2, SubmissionSettled: true},
		{TaskID: "queued", Status: TaskStatusQueued, PollingVersion: 2, SubmissionSettled: true},
		{TaskID: "running", Status: TaskStatusInProgress, PollingVersion: 2, SubmissionSettled: true},
		{TaskID: "future", Status: TaskStatusSubmitted, PollingVersion: 2, SubmissionSettled: true, NextPollAt: 200},
		{TaskID: "unbound", Status: TaskStatusNotStart, PollingVersion: 2},
		{TaskID: "unfunded", Status: TaskStatusSubmitted, PollingVersion: 2},
		{TaskID: "success", Status: TaskStatusSuccess, PollingVersion: 2, SubmissionSettled: true},
		{TaskID: "failed", Status: TaskStatusFailure, PollingVersion: 2, SubmissionSettled: true},
		{TaskID: "v1", Status: TaskStatusSubmitted, SubmissionSettled: true},
	}
	require.NoError(t, db.Create(&tasks).Error)
	ready, err := GetReadyTaskPolls(100, 20)
	require.NoError(t, err)
	require.Len(t, ready, 3)
	assert.Equal(t, []string{"ready", "queued", "running"}, []string{ready[0].TaskID, ready[1].TaskID, ready[2].TaskID})
	stale := *ready[0]
	won, err := ClaimTaskPoll(ready[0], 100, 150)
	require.NoError(t, err)
	assert.True(t, won)
	won, err = ClaimTaskPoll(&stale, 100, 150)
	require.NoError(t, err)
	assert.False(t, won)
	stale.Status = TaskStatusFailure
	won, err = SaveTaskPoll(&stale, TaskStatusSubmitted, 0)
	require.NoError(t, err)
	assert.False(t, won, "stale poll must not overwrite the current lease")
}

func taskIntegrationChannelAccounting(t *testing.T, db *gorm.DB) {
	require.NoError(t, migrateTaskSchema())
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelKeyUsage{}, &ChannelUsageDaily{}))
	channel := Channel{Name: "fixture", Key: "fixture-only", Status: common.ChannelStatusEnabled, UsedQuota: 100, QuotaLimit: 1000, QuotaLimitUsed: 100, QuotaLimitMode: ChannelQuotaLimitModeChannel}
	require.NoError(t, db.Create(&channel).Error)
	params := ChannelUsageDeltaParams{ChannelID: channel.Id, QuotaDelta: 40, StandardQuotaDelta: 20, StandardQuotaExplicit: true, Now: time.Unix(1700000000, 0), RequestDelta: 1}
	rollback := errors.New("fixture rollback")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&TaskSettlement{TaskID: 100, Kind: TaskSettlementKindSettle, QuotaDelta: 40, StandardQuotaDelta: 20}).Error; err != nil {
			return err
		}
		if _, err := ApplyChannelUsageDeltaTx(tx, params); err != nil {
			return err
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 100, channel.UsedQuota)
	assert.EqualValues(t, 100, channel.QuotaLimitUsed)
	for _, table := range []any{&TaskSettlement{}, &ChannelUsageDaily{}} {
		var count int64
		require.NoError(t, db.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := ApplyChannelUsageDeltaTx(tx, params)
		return err
	}))
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 120, channel.UsedQuota, "channel uses standard quota, not customer quota")
	assert.EqualValues(t, 120, channel.QuotaLimitUsed)
	var daily ChannelUsageDaily
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&daily).Error)
	assert.EqualValues(t, 20, daily.Quota)
	assert.EqualValues(t, 1, daily.RequestCount)
	params.QuotaDelta, params.StandardQuotaDelta, params.RequestDelta = -40, -20, 0
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := ApplyChannelUsageDeltaTx(tx, params)
		return err
	}))
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 100, channel.UsedQuota)
	assert.EqualValues(t, 100, channel.QuotaLimitUsed)
	require.NoError(t, db.First(&daily, daily.Id).Error)
	assert.Zero(t, daily.Quota)
	assert.EqualValues(t, 1, daily.RequestCount)
}
