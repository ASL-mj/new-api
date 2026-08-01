package service

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func resetSystemEventWriterTestState() {
	systemEventWriterOnce = sync.Once{}
	systemEventQueue = nil
	systemEventQueued.Store(0)
	systemEventWritten.Store(0)
	systemEventDropped.Store(0)
	systemEventWriteFailed.Store(0)
	systemEventBuffered.Store(0)
	systemEventInfoSequence.Store(0)
	insertSystemEventLogs = model.InsertSystemEventLogs
}

func TestSystemEventWriterStatsExposePendingAndCapacity(t *testing.T) {
	resetSystemEventWriterTestState()
	t.Cleanup(resetSystemEventWriterTestState)

	assert.Equal(t, SystemEventWriterStats{}, GetSystemEventWriterStats())

	systemEventQueue = make(chan model.SystemEventLog, 3)
	systemEventQueue <- model.SystemEventLog{Level: "warn"}
	systemEventQueued.Store(7)
	systemEventWritten.Store(5)
	systemEventDropped.Store(2)
	systemEventWriteFailed.Store(1)

	stats := GetSystemEventWriterStats()
	assert.EqualValues(t, 7, stats.QueuedCount)
	assert.EqualValues(t, 5, stats.WrittenCount)
	assert.EqualValues(t, 2, stats.DroppedCount)
	assert.EqualValues(t, 1, stats.WriteFailedCount)
	assert.Equal(t, 1, stats.PendingCount)
	assert.Equal(t, 3+systemEventBufferSize, stats.Capacity)
}

func TestFlushSystemEventBatchRetainsRowsAfterWriteFailure(t *testing.T) {
	resetSystemEventWriterTestState()
	t.Cleanup(resetSystemEventWriterTestState)

	batch := []model.SystemEventLog{{Id: 1}, {Id: 2}}
	insertSystemEventLogs = func(rows []model.SystemEventLog) error {
		return errors.New("database unavailable")
	}

	remaining, succeeded := flushSystemEventBatch(batch)
	assert.False(t, succeeded)
	assert.Equal(t, batch, remaining)
	assert.EqualValues(t, 1, systemEventWriteFailed.Load())
	assert.Zero(t, systemEventWritten.Load())

	insertSystemEventLogs = func(rows []model.SystemEventLog) error { return nil }
	remaining, succeeded = flushSystemEventBatch(remaining)
	assert.True(t, succeeded)
	assert.Empty(t, remaining)
	assert.EqualValues(t, 2, systemEventWritten.Load())
}
