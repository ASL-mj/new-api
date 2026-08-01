package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSystemEventLogsFiltersRequestID(t *testing.T) {
	prepareMonitorTables(t)
	require.NoError(t, DB.AutoMigrate(&SystemEventLog{}))
	require.NoError(t, InsertSystemEventLogs([]SystemEventLog{
		{CreatedAt: 100, Level: "info", Component: "relay", Message: "first", RequestId: "req-a"},
		{CreatedAt: 101, Level: "error", Component: "relay", Message: "second", RequestId: "req-b"},
	}))

	rows, total, err := GetSystemEventLogs(SystemEventLogQuery{RequestId: "req-b"}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	assert.Equal(t, "second", rows[0].Message)
}
