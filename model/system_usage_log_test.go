package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecordSystemUsageLogIsNonBillableAndTraceable(t *testing.T) {
	previousLogDB := LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	LOG_DB = db
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&Log{}))

	require.NoError(t, RecordSystemUsageLog(RecordSystemUsageLogParams{
		ChannelId:        7,
		PromptTokens:     12,
		CompletionTokens: 3,
		ModelName:        "gpt-5.4-mini",
		TokenName:        "渠道探测",
		Quota:            15,
		Content:          "渠道探测成功",
		UseTimeSeconds:   2,
		Group:            "default",
		RequestId:        "monitor-test-1",
		Other: map[string]interface{}{
			"monitor_probe": true,
			"billing_scope": "standard",
		},
	}))

	var got Log
	require.NoError(t, db.First(&got).Error)
	require.Equal(t, LogTypeSystem, got.Type)
	require.Zero(t, got.UserId)
	require.Equal(t, 7, got.ChannelId)
	require.Equal(t, "monitor-test-1", got.RequestId)
	require.Contains(t, got.Other, "monitor_probe")
}
