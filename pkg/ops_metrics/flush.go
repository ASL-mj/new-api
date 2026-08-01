package opsmetrics

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/monitoring_setting"
)

var (
	initOnce sync.Once

	queryFlushMu sync.RWMutex

	startFlushFn = func() {
		go flushLoop()
	}
	upsertOpsMetrics        = model.UpsertOpsMetrics
	deleteExpiredOpsMetrics = model.DeleteExpiredOpsMetrics
)

func Init() {
	initOnce.Do(startFlushFn)
}

func flushLoop() {
	wasFailing := false
	for {
		time.Sleep(time.Duration(monitoring_setting.GetOpsFlushIntervalSeconds()) * time.Second)
		if !monitoring_setting.GetMonitoringSetting().OpsEnabled {
			continue
		}
		flushSucceeded := flushCompletedBuckets()
		if flushSucceeded {
			common.MarkJobHeartbeat("ops_metrics_flush", "ok", "")
			if wasFailing {
				recordOpsSystemEvent("info", "运维指标刷盘已恢复")
				wasFailing = false
			}
		} else {
			common.MarkJobHeartbeat("ops_metrics_flush", "error", "metrics flush failed; counters restored")
			if !wasFailing {
				recordOpsSystemEvent("error", "运维指标刷盘失败，内存计数已保留并将自动重试")
				wasFailing = true
			}
		}
		if flushSucceeded && monitoring_setting.GetOpsRetentionDays() > 0 {
			cleanupExpiredMetrics(monitoring_setting.GetOpsRetentionDays())
		}
	}
}

func flushCompletedBuckets() bool {
	currentBucketTs := alignTimestamp(nowFunc().Unix())
	flushSucceeded := true

	queryFlushMu.Lock()
	defer queryFlushMu.Unlock()

	hotBuckets.Range(func(key, value any) bool {
		bucketKey := key.(bucketKey)
		if bucketKey.bucketTs >= currentBucketTs {
			return true
		}

		bucket := value.(*hotBucket)
		drained := bucket.closeAndDrain()
		hotBuckets.CompareAndDelete(key, value)
		if drained.data.requestCount == 0 {
			return true
		}

		metric := model.OpsMetricBucket{
			BucketTs:             bucketKey.bucketTs,
			ModelName:            bucketKey.model,
			Group:                bucketKey.group,
			ChannelId:            bucketKey.channelId,
			ChannelType:          bucketKey.channelType,
			RequestCount:         drained.data.requestCount,
			SuccessCount:         drained.data.successCount,
			BusinessLimitedCount: drained.data.businessLimitedCount,
			UpstreamErrorCount:   drained.data.upstreamErrorCount,
			Upstream429Count:     drained.data.upstream429Count,
			Upstream529Count:     drained.data.upstream529Count,
			TotalLatencyMs:       drained.data.totalLatencyMs,
			TtftSumMs:            drained.data.ttftSumMs,
			TtftCount:            drained.data.ttftCount,
			OutputTokens:         drained.data.outputTokens,
			GenerationMs:         drained.data.generationMs,
		}
		if err := upsertOpsMetrics(metric, histogramRows(bucketKey, drained)); err != nil {
			restoreDrainedBucket(bucketKey, drained)
			common.SysError("failed to flush ops metrics bucket: " + err.Error())
			flushSucceeded = false
		}
		return true
	})

	return flushSucceeded
}

func histogramRows(key bucketKey, drained drainedBucket) []model.OpsMetricHistogram {
	rows := make([]model.OpsMetricHistogram, 0, len(drained.duration)+len(drained.ttft))
	for upperBound, count := range drained.duration {
		rows = append(rows, model.OpsMetricHistogram{
			BucketTs:     key.bucketTs,
			Metric:       "duration",
			Group:        key.group,
			ChannelId:    key.channelId,
			ChannelType:  key.channelType,
			UpperBoundMs: upperBound,
			Count:        count,
		})
	}
	for upperBound, count := range drained.ttft {
		rows = append(rows, model.OpsMetricHistogram{
			BucketTs:     key.bucketTs,
			Metric:       "ttft",
			Group:        key.group,
			ChannelId:    key.channelId,
			ChannelType:  key.channelType,
			UpperBoundMs: upperBound,
			Count:        count,
		})
	}
	return rows
}

func restoreDrainedBucket(key bucketKey, drained drainedBucket) {
	if drained.data.requestCount == 0 {
		return
	}
	for {
		current, ok := hotBuckets.Load(key)
		if ok {
			if current.(*hotBucket).addDrained(drained) {
				return
			}
			hotBuckets.CompareAndDelete(key, current)
			continue
		}

		replacement := newHotBucketWithDrained(drained)
		actual, loaded := hotBuckets.LoadOrStore(key, replacement)
		if !loaded {
			return
		}
		if actual.(*hotBucket).addDrained(drained) {
			return
		}
		hotBuckets.CompareAndDelete(key, actual)
	}
}

func cleanupExpiredMetrics(retentionDays int) {
	expireBefore := alignTimestamp(nowFunc().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix())
	if _, err := deleteExpiredOpsMetrics(expireBefore); err != nil {
		common.SysError("failed to cleanup expired ops metrics: " + err.Error())
	}
}
