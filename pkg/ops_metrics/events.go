package opsmetrics

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
)

var (
	systemEventRecorderMu sync.RWMutex
	systemEventRecorder   func(model.SystemEventLog)
)

// SetSystemEventRecorder lets the application wire structured event storage
// without making this metrics package depend on the higher-level service package.
func SetSystemEventRecorder(recorder func(model.SystemEventLog)) {
	systemEventRecorderMu.Lock()
	defer systemEventRecorderMu.Unlock()
	systemEventRecorder = recorder
}

func recordOpsSystemEvent(level, messageKey, message string) {
	systemEventRecorderMu.RLock()
	recorder := systemEventRecorder
	systemEventRecorderMu.RUnlock()
	if recorder == nil {
		return
	}
	recorder(model.SystemEventLog{
		CreatedAt: common.GetTimestamp(), Level: level, Component: "ops_metrics",
		Message: message, MessageKey: messageKey,
	})
}

var (
	opsFlushRecoveredMessageKey = i18n.MsgSystemEventOpsFlushRecovered
	opsFlushFailedMessageKey    = i18n.MsgSystemEventOpsFlushFailed
)
