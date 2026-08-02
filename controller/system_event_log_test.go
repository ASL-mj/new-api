package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	backendI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSystemEventLogsRejectsInvalidTimestamps(t *testing.T) {
	prepareMonitorRunnerTables(t)

	for _, target := range []string{
		"/api/system_event_log/?start_timestamp=bad",
		"/api/system_event_log/?end_timestamp=bad",
		"/api/system_event_log/?start_timestamp=200&end_timestamp=100",
	} {
		recorder := performMonitorGroupRequest(t, http.MethodGet, target, "", GetSystemEventLogs)
		response := decodeMonitorGroupResponse(t, recorder)
		assert.False(t, response["success"].(bool), target)
	}
}

func TestGetSystemEventLogsLocalizesStructuredRowsAndPreservesLegacyText(t *testing.T) {
	prepareMonitorRunnerTables(t)
	require.NoError(t, backendI18n.Init())
	require.NoError(t, model.DB.AutoMigrate(&model.SystemEventLog{}))
	extra, err := common.Marshal(map[string]any{
		"KeyIndex": 2, "QuotaLimitUsed": 500000, "QuotaLimit": 500000,
	})
	require.NoError(t, err)
	require.NoError(t, model.InsertSystemEventLogs([]model.SystemEventLog{
		{CreatedAt: 100, Level: "info", Component: "legacy", Message: "历史原文"},
		{
			CreatedAt: 101, Level: "warn", Component: "channel_usage",
			Message: "渠道 Key 已因额度耗尽自动禁用", MessageKey: backendI18n.MsgSystemEventKeyQuotaExhausted,
			ChannelId: 9, Extra: string(extra),
		},
	}))

	english := performSystemEventLogRequest(t, "en")
	assert.Contains(t, english.Body.String(), "Channel #9 key #2 was automatically disabled")
	assert.Contains(t, english.Body.String(), "历史原文")

	traditional := performSystemEventLogRequest(t, "zh-TW")
	assert.Contains(t, traditional.Body.String(), "管道 #9 的第 2 個密鑰已因額度耗盡自動停用")
	assert.Contains(t, traditional.Body.String(), "历史原文")
}

func performSystemEventLogRequest(t *testing.T, language string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/system_event_log/?p=1&page_size=20", nil)
	context.Request.Header.Set("Accept-Language", language)
	GetSystemEventLogs(context)
	return recorder
}
