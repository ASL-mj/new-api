package perfmetrics

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestRecordPerfSystemEventPersistsMessageKey(t *testing.T) {
	var recorded model.SystemEventLog
	SetSystemEventRecorder(func(event model.SystemEventLog) { recorded = event })
	t.Cleanup(func() { SetSystemEventRecorder(nil) })

	recordPerfSystemEvent("error", perfFlushFailedMessageKey, "fallback")

	assert.Equal(t, "perf_metrics", recorded.Component)
	assert.Equal(t, "system_event.perf_flush_failed", recorded.MessageKey)
	assert.Equal(t, "fallback", recorded.Message)
}
