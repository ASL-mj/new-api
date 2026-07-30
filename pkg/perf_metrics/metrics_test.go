package perfmetrics

import (
	"runtime"
	"sync"
	"sync/atomic"
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

var perfMetricsTestMu sync.Mutex

func preparePerfMetricsTestState(t *testing.T, now time.Time) {
	t.Helper()
	// This package mutates global state; keep tests serialized even if future
	// edits add t.Parallel by mistake.
	perfMetricsTestMu.Lock()

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
	previousGetPerfMetricSummaryBuckets := getPerfMetricSummaryBucketsByRange
	previousDeleteExpiredPerfMetrics := deleteExpiredPerfMetrics
	previousStartFlushFn := startFlushFn

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
	getPerfMetricSummaryBucketsByRange = model.GetPerfMetricSummaryBucketsByRange
	deleteExpiredPerfMetrics = model.DeleteExpiredPerfMetrics
	startFlushFn = func() {
		go flushLoop()
	}
	initOnce = sync.Once{}
	queryFlushMu = sync.RWMutex{}
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
		getPerfMetricSummaryBucketsByRange = previousGetPerfMetricSummaryBuckets
		deleteExpiredPerfMetrics = previousDeleteExpiredPerfMetrics
		startFlushFn = previousStartFlushFn
		initOnce = sync.Once{}
		queryFlushMu = sync.RWMutex{}
		hotBuckets = sync.Map{}
		_ = sqlDB.Close()
		perfMetricsTestMu.Unlock()
	})
}

func insertPerfMetric(t *testing.T, metric *model.PerfMetric) {
	t.Helper()
	require.NoError(t, model.UpsertPerfMetric(metric))
}

func readPersistedCounters(t *testing.T, modelName string) counters {
	t.Helper()
	rows, err := model.GetPerfMetricsByRange(modelName, "", 0, 0)
	require.NoError(t, err)

	var total counters
	for _, row := range rows {
		total.add(counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			ttftSumMs:      row.TtftSumMs,
			ttftCount:      row.TtftCount,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		})
	}
	return total
}

func TestNormalizeHoursBounds(t *testing.T) {
	assert.Equal(t, 24, normalizeHours(0))
	assert.Equal(t, 24, normalizeHours(-3))
	assert.Equal(t, 24, normalizeHours(24))
	assert.Equal(t, 168, normalizeHours(999))
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

func TestQuerySummaryAllMergesPersistedAndHotBuckets(t *testing.T) {
	now := time.Unix(3600, 0).UTC()
	preparePerfMetricsTestState(t, now)

	insertPerfMetric(t, &model.PerfMetric{
		ModelName:      "gpt-5.4",
		Group:          "default",
		BucketTs:       3000,
		RequestCount:   2,
		SuccessCount:   2,
		TotalLatencyMs: 1000,
		OutputTokens:   80,
		GenerationMs:   2000,
	})
	insertPerfMetric(t, &model.PerfMetric{
		ModelName:      "gpt-5.4",
		Group:          "default",
		BucketTs:       3300,
		RequestCount:   1,
		SuccessCount:   0,
		TotalLatencyMs: 2000,
		OutputTokens:   20,
		GenerationMs:   1000,
	})
	Record(Sample{
		Model:        "gpt-5.4",
		Group:        "default",
		LatencyMs:    1000,
		Success:      true,
		OutputTokens: 100,
		GenerationMs: 2000,
	})
	Record(Sample{Model: "hidden-model", Group: "disabled", LatencyMs: 50, Success: true})

	result, err := QuerySummaryAll(1, []string{"default", "auto"})
	require.NoError(t, err)
	require.Len(t, result.Models, 1)

	got := result.Models[0]
	assert.Equal(t, "gpt-5.4", got.ModelName)
	assert.EqualValues(t, 1000, got.AvgLatencyMs)
	assert.Equal(t, 75.0, got.SuccessRate)
	assert.Equal(t, 40.0, got.AvgTps)
	assert.Equal(t, []float64{100, 0, 100}, got.RecentSuccessRates)
}

func TestQuerySummaryAllOmitsModelsWithoutRequests(t *testing.T) {
	preparePerfMetricsTestState(t, time.Unix(3600, 0).UTC())
	insertPerfMetric(t, &model.PerfMetric{
		ModelName:    "empty",
		Group:        "default",
		BucketTs:     3300,
		RequestCount: 0,
	})

	result, err := QuerySummaryAll(24, []string{"default"})
	require.NoError(t, err)
	assert.Empty(t, result.Models)
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
	assert.EqualValues(t, 300, result.Groups[0].Series[1].Ts-result.Groups[0].Series[0].Ts)
	assert.EqualValues(t, 300, result.Groups[0].Series[2].Ts-result.Groups[0].Series[1].Ts)
	assert.EqualValues(t, 3, result.Overall.RequestCount)
}

func TestRecordUsesFixedThreeHundredSecondSourceBucket(t *testing.T) {
	now := time.Unix(3671, 0).UTC()
	preparePerfMetricsTestState(t, now)

	Record(Sample{
		Model:     "gpt-4o",
		Group:     "alpha",
		LatencyMs: 100,
		Success:   true,
	})

	result, err := Query(QueryParams{Model: "gpt-4o", Hours: 1})
	require.NoError(t, err)
	require.Len(t, result.Groups, 1)
	require.Len(t, result.Groups[0].Series, 1)
	assert.EqualValues(t, 3600, result.Groups[0].Series[0].Ts)
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
	bucket := newHotBucket()
	require.True(t, bucket.add(Sample{
		Model:     "gpt-4o",
		Group:     "alpha",
		LatencyMs: 300,
		Success:   true,
	}))
	hotBuckets.Store(key, bucket)

	upsertPerfMetric = func(metric *model.PerfMetric) error {
		replacement := newHotBucket()
		require.True(t, replacement.add(Sample{
			Model:        "gpt-4o",
			Group:        "alpha",
			LatencyMs:    100,
			TtftMs:       25,
			HasTtft:      true,
			Success:      true,
			OutputTokens: 7,
			GenerationMs: 11,
		}))
		hotBuckets.Store(key, replacement)
		return assert.AnError
	}

	flushCompletedBuckets()

	stored, ok := hotBuckets.Load(key)
	require.True(t, ok)
	snapshot := stored.(*hotBucket).snapshot()
	assert.EqualValues(t, 2, snapshot.requestCount)
	assert.EqualValues(t, 2, snapshot.successCount)
	assert.EqualValues(t, 400, snapshot.totalLatencyMs)
	assert.EqualValues(t, 25, snapshot.ttftSumMs)
	assert.EqualValues(t, 1, snapshot.ttftCount)
	assert.EqualValues(t, 7, snapshot.outputTokens)
	assert.EqualValues(t, 11, snapshot.generationMs)

	upsertPerfMetric = model.UpsertPerfMetric
	flushCompletedBuckets()

	_, ok = hotBuckets.Load(key)
	assert.False(t, ok)

	rows, err := model.GetPerfMetricsByRange("gpt-4o", "alpha", 600, 600)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 2, rows[0].RequestCount)
	assert.EqualValues(t, 2, rows[0].SuccessCount)
	assert.EqualValues(t, 400, rows[0].TotalLatencyMs)
	assert.EqualValues(t, 25, rows[0].TtftSumMs)
	assert.EqualValues(t, 1, rows[0].TtftCount)
	assert.EqualValues(t, 7, rows[0].OutputTokens)
	assert.EqualValues(t, 11, rows[0].GenerationMs)
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

func TestQueryWaitsForFlushBoundaryAndStaysConsistent(t *testing.T) {
	now := time.Unix(900, 0).UTC()
	preparePerfMetricsTestState(t, now)

	key := bucketKey{
		model:    "gpt-4o",
		group:    "alpha",
		bucketTs: 600,
	}
	bucket := newHotBucket()
	require.True(t, bucket.add(Sample{
		Model:     "gpt-4o",
		Group:     "alpha",
		LatencyMs: 120,
		Success:   true,
	}))
	hotBuckets.Store(key, bucket)

	dbReadStarted := make(chan struct{})
	releaseDBRead := make(chan struct{})
	getPerfMetricsByRange = func(modelName string, group string, startBucketTs int64, endBucketTs int64) ([]*model.PerfMetric, error) {
		close(dbReadStarted)
		<-releaseDBRead
		return nil, nil
	}

	type queryOutcome struct {
		result QueryResult
		err    error
	}
	queryDone := make(chan queryOutcome, 1)
	go func() {
		result, err := Query(QueryParams{Model: "gpt-4o", Group: "alpha", Hours: 1})
		queryDone <- queryOutcome{result: result, err: err}
	}()

	<-dbReadStarted

	flushDone := make(chan struct{})
	go func() {
		flushCompletedBuckets()
		close(flushDone)
	}()

	select {
	case <-flushDone:
		t.Fatal("flush should wait until query releases the read lock")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseDBRead)
	outcome := <-queryDone
	require.NoError(t, outcome.err)
	assert.EqualValues(t, 1, outcome.result.Overall.RequestCount)

	<-flushDone
	getPerfMetricsByRange = model.GetPerfMetricsByRange

	resultAfterFlush, err := Query(QueryParams{Model: "gpt-4o", Group: "alpha", Hours: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 1, resultAfterFlush.Overall.RequestCount)
}

func TestRecordAndFlushStressPreservesCounters(t *testing.T) {
	nowUnix := atomic.Int64{}
	nowUnix.Store(600)
	preparePerfMetricsTestState(t, time.Unix(nowUnix.Load(), 0).UTC())
	nowFunc = func() time.Time {
		return time.Unix(nowUnix.Load(), 0).UTC()
	}

	const (
		writers          = 8
		iterations       = 1000
		latencyMs  int64 = 13
		ttftMs     int64 = 3
		tokenCount int64 = 7
		genMs      int64 = 11
	)

	stopFlush := make(chan struct{})
	flushWG := sync.WaitGroup{}
	flushWG.Add(1)
	go func() {
		defer flushWG.Done()
		for {
			select {
			case <-stopFlush:
				return
			default:
			}
			nowUnix.Add(300)
			flushCompletedBuckets()
			runtime.Gosched()
			time.Sleep(100 * time.Microsecond)
		}
	}()

	recordWG := sync.WaitGroup{}
	recordWG.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer recordWG.Done()
			for j := 0; j < iterations; j++ {
				Record(Sample{
					Model:        "gpt-4o",
					Group:        "alpha",
					LatencyMs:    latencyMs,
					TtftMs:       ttftMs,
					HasTtft:      true,
					Success:      true,
					OutputTokens: tokenCount,
					GenerationMs: genMs,
				})
				if j%25 == 0 {
					runtime.Gosched()
				}
			}
		}()
	}
	recordWG.Wait()
	close(stopFlush)
	flushWG.Wait()

	nowUnix.Add(300)
	flushCompletedBuckets()
	nowUnix.Add(300)
	flushCompletedBuckets()

	expectedRequests := int64(writers * iterations)
	persisted := readPersistedCounters(t, "gpt-4o")
	assert.EqualValues(t, expectedRequests, persisted.requestCount)
	assert.EqualValues(t, expectedRequests, persisted.successCount)
	assert.EqualValues(t, expectedRequests*latencyMs, persisted.totalLatencyMs)
	assert.EqualValues(t, expectedRequests*ttftMs, persisted.ttftSumMs)
	assert.EqualValues(t, expectedRequests, persisted.ttftCount)
	assert.EqualValues(t, expectedRequests*tokenCount, persisted.outputTokens)
	assert.EqualValues(t, expectedRequests*genMs, persisted.generationMs)

	queryResult, err := Query(QueryParams{Model: "gpt-4o", Group: "alpha", Hours: 168})
	require.NoError(t, err)
	assert.EqualValues(t, expectedRequests, queryResult.Overall.RequestCount)
	assert.EqualValues(t, latencyMs, queryResult.Overall.AvgLatencyMs)
	assert.EqualValues(t, ttftMs, queryResult.Overall.AvgTtftMs)
	assert.Equal(t, 100.0, queryResult.Overall.SuccessRate)
	assert.Equal(t, 636.36, queryResult.Overall.AvgTps)
}

func TestInitIsIdempotent(t *testing.T) {
	preparePerfMetricsTestState(t, time.Unix(3600, 0).UTC())

	var starts atomic.Int64
	startFlushFn = func() {
		starts.Add(1)
	}
	initOnce = sync.Once{}

	Init()
	Init()
	Init()

	assert.EqualValues(t, 1, starts.Load())
}
