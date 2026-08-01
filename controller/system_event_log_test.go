package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
