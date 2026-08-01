package perfmetrics

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var (
	systemEventRecorderMu sync.RWMutex
	systemEventRecorder   func(model.SystemEventLog)
)

// SetSystemEventRecorder wires structured event storage from the application
// layer without introducing a perfmetrics -> service import cycle.
func SetSystemEventRecorder(recorder func(model.SystemEventLog)) {
	systemEventRecorderMu.Lock()
	defer systemEventRecorderMu.Unlock()
	systemEventRecorder = recorder
}

func recordPerfSystemEvent(level, message string) {
	systemEventRecorderMu.RLock()
	recorder := systemEventRecorder
	systemEventRecorderMu.RUnlock()
	if recorder == nil {
		return
	}
	recorder(model.SystemEventLog{
		CreatedAt: common.GetTimestamp(), Level: level, Component: "perf_metrics", Message: message,
	})
}
