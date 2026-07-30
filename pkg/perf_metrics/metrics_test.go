package perfmetrics

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func preparePerfMetricsTestState(t *testing.T, now time.Time) {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	previousNowFunc := nowFunc
	previousUpsertPerfMetric := upsertPerfMetric
	previousGetPerfMetrics := getPerfMetricsByRange
	previousDeleteExpiredPerfMetrics := deleteExpiredPerfMetrics

	isolatedDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := isolatedDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB = isolatedDB
	model.LOG_DB = isolatedDB
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.RDB = nil
	nowFunc = func() time.Time { return now }
	upsertPerfMetric = model.UpsertPerfMetric
	getPerfMetricsByRange = model.GetPerfMetricsByRange
	deleteExpiredPerfMetrics = model.DeleteExpiredPerfMetrics
	hotBuckets = sync.Map{}

	require.NoError(t, isolatedDB.AutoMigrate(&model.PerfMetric{}))

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
		nowFunc = previousNowFunc
		upsertPerfMetric = previousUpsertPerfMetric
		getPerfMetricsByRange = previousGetPerfMetrics
		deleteExpiredPerfMetrics = previousDeleteExpiredPerfMetrics
		hotBuckets = sync.Map{}
		_ = sqlDB.Close()
	})
}

func insertPerfMetric(t *testing.T, metric *model.PerfMetric) {
	t.Helper()
	require.NoError(t, model.UpsertPerfMetric(metric))
}

func TestQueryComputesWeightedAveragesAndRequestCounts(t *testing.T) {
	preparePerfMetricsTestState(t, time.Unix(3600, 0).UTC())

	Record(Sample{
		Model:        "gpt-4o",
		Group:        "alpha",
		LatencyMs:    1000,
		TtftMs:       100,
		HasTtft:      true,
		Success:      true,
		OutputTokens: 100,
		GenerationMs: 2000,
	})
	Record(Sample{
		Model:        "gpt-4o",
		Group:        "alpha",
		LatencyMs:    2000,
		HasTtft:      false,
		Success:      false,
		OutputTokens: 100,
		GenerationMs: 2000,
	})

	result, err := Query(QueryParams{Model: "gpt-4o", Hours: 1})
	require.NoError(t, err)

	require.Equal(t, "gpt-4o", result.ModelName)
	require.Equal(t, 1, result.Hours)
	require.Len(t, result.Groups, 1)
	require.Len(t, result.Overall.Series, 1)

	group := result.Groups[0]
	point := group.Series[0]

	assert.EqualValues(t, 2, result.Overall.RequestCount)
	assert.EqualValues(t, 2, group.RequestCount)
	assert.EqualValues(t, 2, point.RequestCount)
	assert.EqualValues(t, 1500, result.Overall.AvgLatencyMs)
	assert.EqualValues(t, 1500, group.AvgLatencyMs)
	assert.EqualValues(t, 100, result.Overall.AvgTtftMs)
	assert.EqualValues(t, 100, group.AvgTtftMs)
	assert.Equal(t, 50.0, result.Overall.SuccessRate)
	assert.Equal(t, 50.0, group.SuccessRate)
	assert.Equal(t, 50.0, result.Overall.AvgTps)
	assert.Equal(t, 50.0, group.AvgTps)
	assert.EqualValues(t, 3600, point.Ts)
	assert.EqualValues(t, 1500, point.AvgLatencyMs)
	assert.EqualValues(t, 100, point.AvgTtftMs)
	assert.Equal(t, 50.0, point.SuccessRate)
	assert.Equal(t, 50.0, point.AvgTps)
}

func TestRecordNormalizesEmptyGroupAndIgnoresEmptyModel(t *testing.T) {
	preparePerfMetricsTestState(t, time.Unix(3600, 0).UTC())

	Record(Sample{
		Model:     "",
		Group:     "alpha",
		LatencyMs: 100,
		Success:   true,
	})
	Record(Sample{
		Model:     "gpt-4o",
		Group:     "",
		LatencyMs: -25,
		Success:   true,
	})

	result, err := Query(QueryParams{Model: "gpt-4o", Hours: 1})
	require.NoError(t, err)
	require.Len(t, result.Groups, 1)

	assert.Equal(t, "default", result.Groups[0].Group)
	assert.EqualValues(t, 1, result.Overall.RequestCount)
	assert.EqualValues(t, 0, result.Overall.AvgLatencyMs)
}

func TestRecordRelaySampleComputesStreamingAndFallbackDurations(t *testing.T) {
	now := time.Unix(7200, 0).UTC()
	preparePerfMetricsTestState(t, now)

	RecordRelaySample(&relaycommon.RelayInfo{
		OriginModelName:   "gpt-4o",
		UsingGroup:        "alpha",
		IsStream:          true,
		StartTime:         now.Add(-5 * time.Second),
		FirstResponseTime: now.Add(-4 * time.Second),
	}, true, 100)

	streaming, err := Query(QueryParams{Model: "gpt-4o", Hours: 1})
	require.NoError(t, err)
	require.Len(t, streaming.Groups, 1)
	assert.EqualValues(t, 5000, streaming.Overall.AvgLatencyMs)
	assert.EqualValues(t, 1000, streaming.Overall.AvgTtftMs)
	assert.Equal(t, 25.0, streaming.Overall.AvgTps)

	RecordRelaySample(&relaycommon.RelayInfo{
		OriginModelName: "gpt-4o",
		UsingGroup:      "alpha",
		IsStream:        false,
		StartTime:       now.Add(-2 * time.Second),
	}, true, 100)

	fallback, err := Query(QueryParams{Model: "gpt-4o", Hours: 1, Group: "alpha"})
	require.NoError(t, err)
	assert.EqualValues(t, 2, fallback.Overall.RequestCount)
	assert.EqualValues(t, 3500, fallback.Overall.AvgLatencyMs)
	assert.EqualValues(t, 1000, fallback.Overall.AvgTtftMs)
	assert.Equal(t, 33.33, fallback.Overall.AvgTps)
}

func TestQueryUsesFiveMinuteRollupForOneHour(t *testing.T) {
	now := time.Unix(3600, 0).UTC()
	preparePerfMetricsTestState(t, now)

	insertPerfMetric(t, &model.PerfMetric{
		ModelName:      "gpt-4o",
		Group:          "alpha",
		BucketTs:       3000,
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 300,
	})
	insertPerfMetric(t, &model.PerfMetric{
		ModelName:      "gpt-4o",
		Group:          "alpha",
		BucketTs:       3300,
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 600,
	})
	Record(Sample{
		Model:     "gpt-4o",
		Group:     "alpha",
		LatencyMs: 900,
		Success:   true,
	})

	result, err := Query(QueryParams{Model: "gpt-4o", Hours: 1})
	require.NoError(t, err)
	require.Len(t, result.Groups, 1)
	require.Len(t, result.Groups[0].Series, 3)

	assert.EqualValues(t, 3000, result.Groups[0].Series[0].Ts)
	assert.EqualValues(t, 3300, result.Groups[0].Series[1].Ts)
	assert.EqualValues(t, 3600, result.Groups[0].Series[2].Ts)
	assert.EqualValues(t, 3, result.Overall.RequestCount)
}

func TestQueryUsesHourlyRollupForDayAndWeek(t *testing.T) {
	now := time.Unix(48*3600, 0).UTC()
	preparePerfMetricsTestState(t, now)

	for _, ts := range []int64{now.Unix() - 7200, now.Unix() - 6900, now.Unix() - 3600, now.Unix() - 3300} {
		insertPerfMetric(t, &model.PerfMetric{
			ModelName:    "gpt-4o",
			Group:        "alpha",
			BucketTs:     ts,
			RequestCount: 1,
		})
	}

	for _, hours := range []int{24, 168} {
		result, err := Query(QueryParams{Model: "gpt-4o", Hours: hours})
		require.NoError(t, err)
		require.Len(t, result.Groups, 1)
		require.Len(t, result.Groups[0].Series, 2)

		assert.EqualValues(t, now.Unix()-7200, result.Groups[0].Series[0].Ts)
		assert.EqualValues(t, now.Unix()-3600, result.Groups[0].Series[1].Ts)
		assert.EqualValues(t, 2, result.Groups[0].Series[0].RequestCount)
		assert.EqualValues(t, 2, result.Groups[0].Series[1].RequestCount)
	}
}

func TestOverallUsesRawCountersAndAllowedGroupsExcludesInactive(t *testing.T) {
	now := time.Unix(7200, 0).UTC()
	preparePerfMetricsTestState(t, now)

	insertPerfMetric(t, &model.PerfMetric{
		ModelName:      "gpt-4o",
		Group:          "alpha",
		BucketTs:       now.Unix() - 300,
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 1000,
	})
	insertPerfMetric(t, &model.PerfMetric{
		ModelName:      "gpt-4o",
		Group:          "beta",
		BucketTs:       now.Unix() - 300,
		RequestCount:   9,
		SuccessCount:   0,
		TotalLatencyMs: 18000,
	})

	allGroups, err := Query(QueryParams{Model: "gpt-4o", Hours: 1})
	require.NoError(t, err)
	require.Len(t, allGroups.Groups, 2)
	require.Len(t, allGroups.Overall.Series, 1)

	assert.EqualValues(t, 10, allGroups.Overall.RequestCount)
	assert.EqualValues(t, 1900, allGroups.Overall.AvgLatencyMs)
	assert.Equal(t, 10.0, allGroups.Overall.SuccessRate)
	assert.EqualValues(t, 10, allGroups.Overall.Series[0].RequestCount)

	filtered, err := Query(QueryParams{
		Model:         "gpt-4o",
		Hours:         1,
		AllowedGroups: []string{"alpha"},
	})
	require.NoError(t, err)
	require.Len(t, filtered.Groups, 1)
	assert.Equal(t, "alpha", filtered.Groups[0].Group)
	assert.EqualValues(t, 1, filtered.Overall.RequestCount)
	assert.EqualValues(t, 1000, filtered.Overall.AvgLatencyMs)
	assert.Equal(t, 100.0, filtered.Overall.SuccessRate)
	assert.EqualValues(t, 1, filtered.Overall.Series[0].RequestCount)
}

func TestFlushCompletedBucketsRetainsCountersOnFailure(t *testing.T) {
	now := time.Unix(1200, 0).UTC()
	preparePerfMetricsTestState(t, now)

	key := bucketKey{
		model:    "gpt-4o",
		group:    "alpha",
		bucketTs: 600,
	}
	bucket := &atomicBucket{}
	bucket.add(Sample{
		Model:     "gpt-4o",
		Group:     "alpha",
		LatencyMs: 300,
		Success:   true,
	})
	hotBuckets.Store(key, bucket)

	upsertPerfMetric = func(metric *model.PerfMetric) error {
		return assert.AnError
	}

	flushCompletedBuckets()

	stored, ok := hotBuckets.Load(key)
	require.True(t, ok)
	assert.EqualValues(t, 1, stored.(*atomicBucket).snapshot().requestCount)

	upsertPerfMetric = model.UpsertPerfMetric
	flushCompletedBuckets()

	_, ok = hotBuckets.Load(key)
	assert.False(t, ok)

	rows, err := model.GetPerfMetricsByRange("gpt-4o", "alpha", 600, 600)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 1, rows[0].RequestCount)
}

func TestConcurrentRecordAccumulatesWithoutLoss(t *testing.T) {
	preparePerfMetricsTestState(t, time.Unix(3600, 0).UTC())

	const goroutines = 32
	const perGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				Record(Sample{
					Model:     "gpt-4o",
					Group:     "alpha",
					LatencyMs: 10,
					Success:   true,
				})
			}
		}()
	}
	wg.Wait()

	result, err := Query(QueryParams{Model: "gpt-4o", Hours: 1})
	require.NoError(t, err)
	assert.EqualValues(t, goroutines*perGoroutine, result.Overall.RequestCount)
	assert.EqualValues(t, goroutines*perGoroutine, result.Groups[0].Series[0].RequestCount)
}
