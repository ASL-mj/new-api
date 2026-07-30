package perfmetrics

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

func flushLoop() {
	for {
		time.Sleep(time.Duration(perf_metrics_setting.GetFlushIntervalMinutes()) * time.Minute)

		setting := perf_metrics_setting.GetPerfMetricsSetting()
		if !setting.Enabled {
			continue
		}

		flushCompletedBuckets()
		if setting.RetentionDays > 0 {
			cleanupExpiredMetrics(setting.RetentionDays)
		}
	}
}

func flushCompletedBuckets() {
	currentBucketTs := alignTimestamp(nowFunc().Unix(), int64(perf_metrics_setting.GetBucketSeconds()))
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
			return true
		}

		return true
	})
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
