package perfmetrics

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

func flushLoop() {
	wasFailing := false
	for {
		time.Sleep(time.Duration(perf_metrics_setting.GetFlushIntervalMinutes()) * time.Minute)

		setting := perf_metrics_setting.GetPerfMetricsSetting()
		if !setting.Enabled {
			continue
		}

		flushSucceeded := flushCompletedBuckets()
		if flushSucceeded {
			common.MarkJobHeartbeat("perf_metrics_flush", "ok", "")
			if wasFailing {
				recordPerfSystemEvent("info", perfFlushRecoveredMessageKey, "模型性能指标刷盘已恢复")
				wasFailing = false
			}
		} else {
			common.MarkJobHeartbeat("perf_metrics_flush", "error", "metrics flush failed; counters restored")
			if !wasFailing {
				recordPerfSystemEvent("error", perfFlushFailedMessageKey, "模型性能指标刷盘失败，内存计数已保留并将自动重试")
				wasFailing = true
			}
		}
		if flushSucceeded && setting.RetentionDays > 0 {
			cleanupExpiredMetrics(setting.RetentionDays)
		}
	}
}

func flushCompletedBuckets() bool {
	currentBucketTs := alignTimestamp(nowFunc().Unix(), int64(perf_metrics_setting.GetBucketSeconds()))
	flushSucceeded := true
	queryFlushMu.Lock()
	defer queryFlushMu.Unlock()

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		bucket := value.(*hotBucket)
		if k.bucketTs >= currentBucketTs {
			return true
		}

		drained := bucket.closeAndDrain()
		hotBuckets.CompareAndDelete(key, value)
		if drained.requestCount == 0 {
			return true
		}

		err := upsertPerfMetric(&model.PerfMetric{
			ModelName:      k.model,
			Group:          k.group,
			BucketTs:       k.bucketTs,
			RequestCount:   drained.requestCount,
			SuccessCount:   drained.successCount,
			TotalLatencyMs: drained.totalLatencyMs,
			TtftSumMs:      drained.ttftSumMs,
			TtftCount:      drained.ttftCount,
			OutputTokens:   drained.outputTokens,
			GenerationMs:   drained.generationMs,
		})
		if err != nil {
			restoreDrainedCounters(k, drained)
			common.SysError("failed to flush perf metrics bucket: " + err.Error())
			flushSucceeded = false
			return true
		}

		return true
	})
	return flushSucceeded
}

func restoreDrainedCounters(key bucketKey, drained counters) {
	if drained.requestCount == 0 {
		return
	}

	for {
		current, ok := hotBuckets.Load(key)
		if ok {
			if current.(*hotBucket).addCounters(drained) {
				return
			}
			hotBuckets.CompareAndDelete(key, current)
			continue
		}

		replacement := newHotBucketWithCounters(drained)
		actual, loaded := hotBuckets.LoadOrStore(key, replacement)
		if !loaded {
			return
		}
		if actual.(*hotBucket).addCounters(drained) {
			return
		}
		hotBuckets.CompareAndDelete(key, actual)
	}
}

func cleanupExpiredMetrics(retentionDays int) {
	expireBefore := alignTimestamp(nowFunc().Add(-time.Duration(retentionDays)*24*time.Hour).Unix(), int64(perf_metrics_setting.GetBucketSeconds()))
	if _, err := deleteExpiredPerfMetrics(expireBefore); err != nil {
		common.SysError("failed to cleanup expired perf metrics: " + err.Error())
	}
}
