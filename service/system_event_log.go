package service

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/monitoring_setting"
)

const (
	systemEventQueueSize  = 2048
	systemEventBatchSize  = 100
	systemEventFlushEvery = time.Second
)

type SystemEventWriterStats struct {
	QueuedCount      int64 `json:"queued_count"`
	WrittenCount     int64 `json:"written_count"`
	DroppedCount     int64 `json:"dropped_count"`
	WriteFailedCount int64 `json:"write_failed_count"`
	PendingCount     int   `json:"pending_count"`
	Capacity         int   `json:"capacity"`
}

var (
	systemEventWriterOnce   sync.Once
	systemEventQueue        chan model.SystemEventLog
	systemEventQueued       atomic.Int64
	systemEventWritten      atomic.Int64
	systemEventDropped      atomic.Int64
	systemEventWriteFailed  atomic.Int64
	systemEventInfoSequence atomic.Uint64
)

func StartSystemEventWriter() {
	systemEventWriterOnce.Do(func() {
		systemEventQueue = make(chan model.SystemEventLog, systemEventQueueSize)
		go systemEventWriterLoop()
	})
}

func RecordSystemEvent(event model.SystemEventLog) {
	if !monitoring_setting.GetMonitoringSetting().SystemLogEnabled {
		return
	}
	if strings.EqualFold(event.Level, "info") && !shouldRecordSystemInfoEvent() {
		return
	}
	if systemEventQueue == nil {
		StartSystemEventWriter()
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = common.GetTimestamp()
	}
	event = redactSystemEvent(event)
	select {
	case systemEventQueue <- event:
		systemEventQueued.Add(1)
	default:
		systemEventDropped.Add(1)
	}
}

func shouldRecordSystemInfoEvent() bool {
	rate := monitoring_setting.GetSystemLogInfoSampleRate()
	if rate <= 0 {
		return false
	}
	if rate >= 100 {
		return true
	}
	return systemEventInfoSequence.Add(1)%100 < uint64(rate)
}

func GetSystemEventWriterStats() SystemEventWriterStats {
	return SystemEventWriterStats{
		QueuedCount:      systemEventQueued.Load(),
		WrittenCount:     systemEventWritten.Load(),
		DroppedCount:     systemEventDropped.Load(),
		WriteFailedCount: systemEventWriteFailed.Load(),
		PendingCount:     len(systemEventQueue),
		Capacity:         cap(systemEventQueue),
	}
}

func systemEventWriterLoop() {
	ticker := time.NewTicker(systemEventFlushEvery)
	defer ticker.Stop()
	batch := make([]model.SystemEventLog, 0, systemEventBatchSize)
	lastCleanup := time.Time{}
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := model.InsertSystemEventLogs(batch); err != nil {
			systemEventWriteFailed.Add(1)
			common.SysError("failed to write system event logs: " + err.Error())
			batch = batch[:0]
			return
		}
		systemEventWritten.Add(int64(len(batch)))
		batch = batch[:0]
		if lastCleanup.IsZero() || time.Since(lastCleanup) >= 24*time.Hour {
			cleanupExpiredSystemEvents()
			lastCleanup = time.Now()
		}
	}
	for {
		select {
		case event := <-systemEventQueue:
			batch = append(batch, event)
			if len(batch) >= systemEventBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func cleanupExpiredSystemEvents() {
	expireBefore := time.Now().Add(-time.Duration(monitoring_setting.GetSystemLogRetentionDays()) * 24 * time.Hour).Unix()
	if _, err := model.DeleteExpiredSystemEventLogs(expireBefore); err != nil {
		common.SysError("failed to cleanup system event logs: " + err.Error())
	}
}
