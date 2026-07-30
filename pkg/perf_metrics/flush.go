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
	hotBuckets.Range(func(key, value any) bool {
		bucket := key.(bucketKey)
		if bucket.bucketTs >= currentBucketTs {
			return true
		}

		drained := value.(*atomicBucket).drain()
		if drained.requestCount == 0 {
			hotBuckets.Delete(key)
			return true
		}

		err := upsertPerfMetric(&model.PerfMetric{
			ModelName:      bucket.model,
			Group:          bucket.group,
			BucketTs:       bucket.bucketTs,
			RequestCount:   drained.requestCount,
			SuccessCount:   drained.successCount,
			TotalLatencyMs: drained.totalLatencyMs,
			TtftSumMs:      drained.ttftSumMs,
			TtftCount:      drained.ttftCount,
			OutputTokens:   drained.outputTokens,
			GenerationMs:   drained.generationMs,
		})
		if err != nil {
			value.(*atomicBucket).addCounters(drained)
			common.SysError("failed to flush perf metrics bucket: " + err.Error())
			return true
		}

		hotBuckets.Delete(key)
		return true
	})
}

func cleanupExpiredMetrics(retentionDays int) {
	expireBefore := alignTimestamp(nowFunc().Add(-time.Duration(retentionDays)*24*time.Hour).Unix(), int64(perf_metrics_setting.GetBucketSeconds()))
	if _, err := deleteExpiredPerfMetrics(expireBefore); err != nil {
		common.SysError("failed to cleanup expired perf metrics: " + err.Error())
	}
}
