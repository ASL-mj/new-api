package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertOpsMetricsAccumulatesBucketAndHistograms(t *testing.T) {
	prepareMonitorTables(t)
	require.NoError(t, DB.AutoMigrate(&OpsMetricBucket{}, &OpsMetricHistogram{}))

	first := OpsMetricBucket{
		BucketTs:       120,
		ModelName:      "gpt-5.4",
		Group:          "default",
		ChannelId:      2,
		ChannelType:    1,
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 100,
	}
	require.NoError(t, UpsertOpsMetrics(first, []OpsMetricHistogram{{
		BucketTs: 120, Metric: "duration", Group: "default", ChannelId: 2, ChannelType: 1, UpperBoundMs: 100, Count: 1,
	}}))

	second := first
	second.RequestCount = 2
	second.SuccessCount = 1
	second.TotalLatencyMs = 300
	require.NoError(t, UpsertOpsMetrics(second, []OpsMetricHistogram{{
		BucketTs: 120, Metric: "duration", Group: "default", ChannelId: 2, ChannelType: 1, UpperBoundMs: 100, Count: 2,
	}}))

	var bucket OpsMetricBucket
	require.NoError(t, DB.First(&bucket).Error)
	assert.EqualValues(t, 3, bucket.RequestCount)
	assert.EqualValues(t, 2, bucket.SuccessCount)
	assert.EqualValues(t, 400, bucket.TotalLatencyMs)

	var histogram OpsMetricHistogram
	require.NoError(t, DB.First(&histogram).Error)
	assert.EqualValues(t, 3, histogram.Count)
}

func TestUpsertOpsMetricsRejectsInvalidHistogramBeforeWritingBucket(t *testing.T) {
	prepareMonitorTables(t)
	require.NoError(t, DB.AutoMigrate(&OpsMetricBucket{}, &OpsMetricHistogram{}))

	err := UpsertOpsMetrics(OpsMetricBucket{
		BucketTs: 120, ModelName: "gpt-5.4", Group: "default", ChannelId: 2, ChannelType: 1, RequestCount: 1,
	}, []OpsMetricHistogram{{Metric: "invalid", Count: 1}})
	require.Error(t, err)

	var count int64
	require.NoError(t, DB.Model(&OpsMetricBucket{}).Count(&count).Error)
	assert.Zero(t, count)
}
