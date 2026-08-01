package opsmetrics

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/setting/monitoring_setting"
)

const opsBucketSeconds int64 = 60

type bucketKey struct {
	bucketTs               int64
	model, group           string
	channelId, channelType int
}

type counters struct {
	requestCount         int64
	successCount         int64
	businessLimitedCount int64
	upstreamErrorCount   int64
	upstream429Count     int64
	upstream529Count     int64
	totalLatencyMs       int64
	ttftSumMs            int64
	ttftCount            int64
	outputTokens         int64
	generationMs         int64
}

type drainedBucket struct {
	data     counters
	duration map[int64]int64
	ttft     map[int64]int64
}

type hotBucket struct {
	mu             sync.Mutex
	data           counters
	duration, ttft map[int64]int64
	closed         bool
}

var (
	hotBuckets sync.Map
	nowFunc    = time.Now
)

func newHotBucket() *hotBucket {
	return &hotBucket{}
}

func newHotBucketWithDrained(drained drainedBucket) *hotBucket {
	return &hotBucket{
		data:     drained.data,
		duration: cloneHistogram(drained.duration),
		ttft:     cloneHistogram(drained.ttft),
	}
}

func (b *hotBucket) add(sample Sample) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	b.data.requestCount++
	if sample.Success {
		b.data.successCount++
	} else {
		switch ClassifyError(sample) {
		case ErrorClassBusinessLimited:
			b.data.businessLimitedCount++
		case ErrorClassUpstream:
			b.data.upstreamErrorCount++
			if sample.StatusCode == 429 {
				b.data.upstream429Count++
			}
			if sample.StatusCode == 529 {
				b.data.upstream529Count++
			}
		}
	}
	b.data.totalLatencyMs += sample.LatencyMs
	b.data.outputTokens += sample.OutputTokens
	b.data.generationMs += sample.GenerationMs
	if b.duration == nil {
		b.duration = make(map[int64]int64)
	}
	b.duration[histogramUpperBound(sample.LatencyMs)]++
	if sample.HasTtft {
		b.data.ttftSumMs += sample.TtftMs
		b.data.ttftCount++
		if b.ttft == nil {
			b.ttft = make(map[int64]int64)
		}
		b.ttft[histogramUpperBound(sample.TtftMs)]++
	}
	return true
}

func (b *hotBucket) snapshot() counters {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data
}

func (b *hotBucket) snapshotWithHistograms() (counters, map[int64]int64, map[int64]int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data, cloneHistogram(b.duration), cloneHistogram(b.ttft)
}

func (b *hotBucket) closeAndDrain() drainedBucket {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	drained := drainedBucket{
		data:     b.data,
		duration: b.duration,
		ttft:     b.ttft,
	}
	b.data = counters{}
	b.duration = nil
	b.ttft = nil
	return drained
}

func (b *hotBucket) addDrained(drained drainedBucket) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	b.data.add(drained.data)
	b.duration = mergeHistogram(b.duration, drained.duration)
	b.ttft = mergeHistogram(b.ttft, drained.ttft)
	return true
}

func (c *counters) add(value counters) {
	c.requestCount += value.requestCount
	c.successCount += value.successCount
	c.businessLimitedCount += value.businessLimitedCount
	c.upstreamErrorCount += value.upstreamErrorCount
	c.upstream429Count += value.upstream429Count
	c.upstream529Count += value.upstream529Count
	c.totalLatencyMs += value.totalLatencyMs
	c.ttftSumMs += value.ttftSumMs
	c.ttftCount += value.ttftCount
	c.outputTokens += value.outputTokens
	c.generationMs += value.generationMs
}

func Record(sample Sample) {
	if !monitoring_setting.GetMonitoringSetting().OpsEnabled || sample.Model == "" || sample.ChannelId < 0 {
		return
	}
	if sample.Group == "" {
		sample.Group = "default"
	}
	if sample.BucketTime.IsZero() {
		sample.BucketTime = nowFunc()
	}
	if sample.LatencyMs < 0 {
		sample.LatencyMs = 0
	}
	if sample.TtftMs < 0 {
		sample.TtftMs = 0
	}
	if sample.OutputTokens < 0 {
		sample.OutputTokens = 0
	}
	if sample.GenerationMs < 0 {
		sample.GenerationMs = 0
	}
	key := bucketKey{bucketTs: alignTimestamp(sample.BucketTime.Unix()), model: sample.Model, group: sample.Group, channelId: sample.ChannelId, channelType: sample.ChannelType}
	for {
		actual, _ := hotBuckets.LoadOrStore(key, newHotBucket())
		if actual.(*hotBucket).add(sample) {
			return
		}
		hotBuckets.CompareAndDelete(key, actual)
	}
}

func alignTimestamp(ts int64) int64 {
	return ts - ts%opsBucketSeconds
}
