package opsmetrics

import (
	"github.com/QuantumNous/new-api/setting/monitoring_setting"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

func TestRecordSeparatesBusinessAndUpstreamFailures(t *testing.T) {
	original := *monitoring_setting.GetMonitoringSetting()
	*monitoring_setting.GetMonitoringSetting() = original
	hotBuckets = sync.Map{}
	t.Cleanup(func() { *monitoring_setting.GetMonitoringSetting() = original; hotBuckets = sync.Map{} })
	now := time.Unix(120, 0)
	originalNow := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = originalNow })
	Record(Sample{Model: "gpt", Group: "default", ChannelId: 1, Success: true, LatencyMs: 50})
	Record(Sample{Model: "gpt", Group: "default", ChannelId: 1, StatusCode: 403, ErrorCode: "insufficient_user_quota", LatencyMs: 20})
	Record(Sample{Model: "gpt", Group: "default", ChannelId: 1, StatusCode: 529, LatencyMs: 30})
	key := bucketKey{bucketTs: 120, model: "gpt", group: "default", channelId: 1}
	value, ok := hotBuckets.Load(key)
	assert.True(t, ok)
	counter := value.(*hotBucket).snapshot()
	assert.EqualValues(t, 3, counter.requestCount)
	assert.EqualValues(t, 1, counter.successCount)
	assert.EqualValues(t, 1, counter.businessLimitedCount)
	assert.EqualValues(t, 1, counter.upstreamErrorCount)
	assert.EqualValues(t, 1, counter.upstream529Count)
}
