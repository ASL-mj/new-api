package opsmetrics

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/monitoring_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var opsMetricsTestMu sync.Mutex

func prepareOpsMetricsTestState(t *testing.T, now time.Time) {
	t.Helper()
	opsMetricsTestMu.Lock()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousNowFunc := nowFunc
	previousUpsertOpsMetrics := upsertOpsMetrics
	previousGetOpsMetricBuckets := getOpsMetricBucketsByRange
	previousDeleteExpiredOpsMetrics := deleteExpiredOpsMetrics
	previousStartFlushFn := startFlushFn
	previousSetting := *monitoring_setting.GetMonitoringSetting()

	isolateDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := isolateDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB = isolateDB
	model.LOG_DB = isolateDB
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	nowFunc = func() time.Time { return now }
	upsertOpsMetrics = model.UpsertOpsMetrics
	getOpsMetricBucketsByRange = model.GetOpsMetricBucketsByRange
	deleteExpiredOpsMetrics = model.DeleteExpiredOpsMetrics
	startFlushFn = func() { go flushLoop() }
	initOnce = sync.Once{}
	queryFlushMu = sync.RWMutex{}
	hotBuckets = sync.Map{}
	setting := previousSetting
	setting.OpsEnabled = true
	setting.OpsRetentionDays = 30
	*monitoring_setting.GetMonitoringSetting() = setting

	require.NoError(t, isolateDB.AutoMigrate(&model.OpsMetricBucket{}, &model.OpsMetricHistogram{}))

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		nowFunc = previousNowFunc
		upsertOpsMetrics = previousUpsertOpsMetrics
		getOpsMetricBucketsByRange = previousGetOpsMetricBuckets
		deleteExpiredOpsMetrics = previousDeleteExpiredOpsMetrics
		startFlushFn = previousStartFlushFn
		initOnce = sync.Once{}
		queryFlushMu = sync.RWMutex{}
		hotBuckets = sync.Map{}
		*monitoring_setting.GetMonitoringSetting() = previousSetting
		_ = sqlDB.Close()
		opsMetricsTestMu.Unlock()
	})
}

func TestFlushCompletedBucketsPersistsCountersAndHistograms(t *testing.T) {
	prepareOpsMetricsTestState(t, time.Unix(120, 0).UTC())

	Record(Sample{
		BucketTime:   time.Unix(60, 0).UTC(),
		Model:        "gpt-5.4",
		Group:        "default",
		ChannelId:    7,
		ChannelType:  1,
		Success:      true,
		LatencyMs:    300,
		TtftMs:       50,
		HasTtft:      true,
		OutputTokens: 10,
		GenerationMs: 100,
	})
	Record(Sample{
		BucketTime:  time.Unix(60, 0).UTC(),
		Model:       "gpt-5.4",
		Group:       "default",
		ChannelId:   7,
		ChannelType: 1,
		StatusCode:  502,
		LatencyMs:   800,
	})

	assert.True(t, flushCompletedBuckets())

	var buckets []model.OpsMetricBucket
	require.NoError(t, model.DB.Find(&buckets).Error)
	require.Len(t, buckets, 1)
	assert.EqualValues(t, 2, buckets[0].RequestCount)
	assert.EqualValues(t, 1, buckets[0].SuccessCount)
	assert.EqualValues(t, 1, buckets[0].UpstreamErrorCount)
	assert.EqualValues(t, 1100, buckets[0].TotalLatencyMs)
	assert.EqualValues(t, 50, buckets[0].TtftSumMs)
	assert.EqualValues(t, 1, buckets[0].TtftCount)
	assert.EqualValues(t, 10, buckets[0].OutputTokens)

	var histograms []model.OpsMetricHistogram
	require.NoError(t, model.DB.Order("metric ASC, upper_bound_ms ASC").Find(&histograms).Error)
	require.Len(t, histograms, 3)
	assert.Equal(t, "duration", histograms[0].Metric)
	assert.EqualValues(t, 500, histograms[0].UpperBoundMs)
	assert.Equal(t, "duration", histograms[1].Metric)
	assert.EqualValues(t, 1000, histograms[1].UpperBoundMs)
	assert.Equal(t, "ttft", histograms[2].Metric)
	assert.EqualValues(t, 100, histograms[2].UpperBoundMs)
	assert.EqualValues(t, 1, histograms[2].Count)
}

func TestFlushCompletedBucketsRestoresCountersAndHistogramsOnFailure(t *testing.T) {
	prepareOpsMetricsTestState(t, time.Unix(180, 0).UTC())

	key := bucketKey{bucketTs: 60, model: "gpt-5.4", group: "default", channelId: 7, channelType: 1}
	Record(Sample{BucketTime: time.Unix(60, 0).UTC(), Model: key.model, Group: key.group, ChannelId: key.channelId, ChannelType: key.channelType, Success: true, LatencyMs: 300})

	upsertOpsMetrics = func(model.OpsMetricBucket, []model.OpsMetricHistogram) error {
		replacement := newHotBucket()
		require.True(t, replacement.add(Sample{Model: key.model, Group: key.group, ChannelId: key.channelId, ChannelType: key.channelType, Success: true, LatencyMs: 100, TtftMs: 25, HasTtft: true}))
		hotBuckets.Store(key, replacement)
		return assert.AnError
	}

	assert.False(t, flushCompletedBuckets())

	upsertOpsMetrics = model.UpsertOpsMetrics
	assert.True(t, flushCompletedBuckets())

	var bucket model.OpsMetricBucket
	require.NoError(t, model.DB.First(&bucket).Error)
	assert.EqualValues(t, 2, bucket.RequestCount)
	assert.EqualValues(t, 2, bucket.SuccessCount)
	assert.EqualValues(t, 400, bucket.TotalLatencyMs)
	assert.EqualValues(t, 25, bucket.TtftSumMs)
	assert.EqualValues(t, 1, bucket.TtftCount)

	var histogram model.OpsMetricHistogram
	require.NoError(t, model.DB.Where("metric = ? AND upper_bound_ms = ?", "ttft", 100).First(&histogram).Error)
	assert.EqualValues(t, 1, histogram.Count)
}

func TestQueryChannelSuccessRateMergesPersistedAndCurrentBucket(t *testing.T) {
	now := time.Unix(180, 0).UTC()
	prepareOpsMetricsTestState(t, now)

	require.NoError(t, model.UpsertOpsMetrics(model.OpsMetricBucket{
		BucketTs:     120,
		ModelName:    "gpt-5.4",
		Group:        "default",
		ChannelId:    7,
		ChannelType:  1,
		RequestCount: 4,
		SuccessCount: 3,
	}, nil))
	Record(Sample{Model: "gpt-5.4", Group: "default", ChannelId: 7, ChannelType: 1, Success: true, LatencyMs: 50})

	rate, err := QueryChannelSuccessRate([]int{7}, time.Hour)
	require.NoError(t, err)
	require.NotNil(t, rate)
	assert.Equal(t, 80.0, *rate)

	empty, err := QueryChannelSuccessRate([]int{8}, time.Hour)
	require.NoError(t, err)
	assert.Nil(t, empty)
}

func TestQueryChannelSuccessRatesAggregatesMultipleMonitorGroupsInOneQuery(t *testing.T) {
	now := time.Unix(180, 0).UTC()
	prepareOpsMetricsTestState(t, now)

	require.NoError(t, model.UpsertOpsMetrics(model.OpsMetricBucket{
		BucketTs: 120, ModelName: "gpt-5.4", Group: "default", ChannelId: 7,
		ChannelType: 1, RequestCount: 4, SuccessCount: 3,
	}, nil))
	require.NoError(t, model.UpsertOpsMetrics(model.OpsMetricBucket{
		BucketTs: 120, ModelName: "gpt-5.4", Group: "default", ChannelId: 8,
		ChannelType: 1, RequestCount: 2, SuccessCount: 2,
	}, nil))

	rates, err := QueryChannelSuccessRates(map[int]ChannelSuccessRateQuery{
		10: {ChannelIDs: []int{7}, Model: "gpt-5.4"},
		20: {ChannelIDs: []int{7, 8}, Model: "gpt-5.4"},
		30: {ChannelIDs: []int{9}, Model: "gpt-5.4"},
	}, time.Hour)
	require.NoError(t, err)
	require.NotNil(t, rates[10])
	require.NotNil(t, rates[20])
	assert.Equal(t, 75.0, *rates[10])
	assert.Equal(t, 83.33, *rates[20])
	assert.Nil(t, rates[30])
}

func TestQueryChannelSuccessRatesFiltersEachGroupByModel(t *testing.T) {
	now := time.Unix(180, 0).UTC()
	prepareOpsMetricsTestState(t, now)
	require.NoError(t, model.UpsertOpsMetrics(model.OpsMetricBucket{
		BucketTs: 120, ModelName: "gpt-5.4", Group: "default", ChannelId: 7,
		ChannelType: 1, RequestCount: 4, SuccessCount: 4,
	}, nil))
	require.NoError(t, model.UpsertOpsMetrics(model.OpsMetricBucket{
		BucketTs: 120, ModelName: "gpt-5.5", Group: "default", ChannelId: 7,
		ChannelType: 1, RequestCount: 4, SuccessCount: 0,
	}, nil))

	rates, err := QueryChannelSuccessRates(map[int]ChannelSuccessRateQuery{
		10: {ChannelIDs: []int{7}, Model: "gpt-5.4"},
		20: {ChannelIDs: []int{7}, Model: "gpt-5.5"},
	}, time.Hour)
	require.NoError(t, err)
	require.NotNil(t, rates[10])
	require.NotNil(t, rates[20])
	assert.Equal(t, 100.0, *rates[10])
	assert.Equal(t, 0.0, *rates[20])
}
