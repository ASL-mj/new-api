package opsmetrics

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestRecordOpsSystemEventPersistsMessageKey(t *testing.T) {
	var recorded model.SystemEventLog
	SetSystemEventRecorder(func(event model.SystemEventLog) { recorded = event })
	t.Cleanup(func() { SetSystemEventRecorder(nil) })

	recordOpsSystemEvent("error", opsFlushFailedMessageKey, "fallback")

	assert.Equal(t, "ops_metrics", recorded.Component)
	assert.Equal(t, "system_event.ops_flush_failed", recorded.MessageKey)
	assert.Equal(t, "fallback", recorded.Message)
}
