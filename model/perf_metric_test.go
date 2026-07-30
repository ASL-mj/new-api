package model

import (
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func preparePerfMetricTable(t *testing.T) {
	t.Helper()

	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	isolatedDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := isolatedDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	DB = isolatedDB
	LOG_DB = isolatedDB
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	require.NoError(t, isolatedDB.AutoMigrate(&PerfMetric{}))

	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		_ = sqlDB.Close()
	})
}

func TestUpsertPerfMetricAccumulatesSingleRow(t *testing.T) {
	preparePerfMetricTable(t)

	first := &PerfMetric{
		ModelName:      "gpt-4o",
		Group:          "alpha",
		BucketTs:       300,
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 120,
		TtftSumMs:      20,
		TtftCount:      1,
		OutputTokens:   100,
		GenerationMs:   90,
	}
	second := &PerfMetric{
		ModelName:      "gpt-4o",
		Group:          "alpha",
		BucketTs:       300,
		RequestCount:   2,
		SuccessCount:   1,
		TotalLatencyMs: 240,
		TtftSumMs:      30,
		TtftCount:      1,
		OutputTokens:   150,
		GenerationMs:   110,
	}

	require.NoError(t, UpsertPerfMetric(first))
	require.NoError(t, UpsertPerfMetric(second))

	var metrics []PerfMetric
	require.NoError(t, DB.Order("id ASC").Find(&metrics).Error)
	require.Len(t, metrics, 1)

	got := metrics[0]
	assert.Equal(t, "gpt-4o", got.ModelName)
	assert.Equal(t, "alpha", got.Group)
	assert.EqualValues(t, 300, got.BucketTs)
	assert.EqualValues(t, 3, got.RequestCount)
	assert.EqualValues(t, 2, got.SuccessCount)
	assert.EqualValues(t, 360, got.TotalLatencyMs)
	assert.EqualValues(t, 50, got.TtftSumMs)
	assert.EqualValues(t, 2, got.TtftCount)
	assert.EqualValues(t, 250, got.OutputTokens)
	assert.EqualValues(t, 200, got.GenerationMs)
}

func TestUpsertPerfMetricSeparatesGroupAndBucket(t *testing.T) {
	preparePerfMetricTable(t)

	require.NoError(t, UpsertPerfMetric(&PerfMetric{
		ModelName:    "gpt-4o",
		Group:        "alpha",
		BucketTs:     300,
		RequestCount: 1,
	}))
	require.NoError(t, UpsertPerfMetric(&PerfMetric{
		ModelName:    "gpt-4o",
		Group:        "beta",
		BucketTs:     300,
		RequestCount: 1,
	}))
	require.NoError(t, UpsertPerfMetric(&PerfMetric{
		ModelName:    "gpt-4o",
		Group:        "alpha",
		BucketTs:     600,
		RequestCount: 1,
	}))

	var metrics []PerfMetric
	require.NoError(t, DB.Find(&metrics).Error)
	require.Len(t, metrics, 3)

	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].Group == metrics[j].Group {
			return metrics[i].BucketTs < metrics[j].BucketTs
		}
		return metrics[i].Group < metrics[j].Group
	})

	assert.Equal(t, "alpha", metrics[0].Group)
	assert.EqualValues(t, 300, metrics[0].BucketTs)
	assert.Equal(t, "alpha", metrics[1].Group)
	assert.EqualValues(t, 600, metrics[1].BucketTs)
	assert.Equal(t, "beta", metrics[2].Group)
	assert.EqualValues(t, 300, metrics[2].BucketTs)
}

func TestGetPerfMetricsByRange(t *testing.T) {
	preparePerfMetricTable(t)

	for _, metric := range []*PerfMetric{
		{ModelName: "gpt-4o", Group: "alpha", BucketTs: 300, RequestCount: 1},
		{ModelName: "gpt-4o", Group: "alpha", BucketTs: 600, RequestCount: 2},
		{ModelName: "gpt-4o", Group: "alpha", BucketTs: 900, RequestCount: 3},
		{ModelName: "gpt-4o", Group: "beta", BucketTs: 600, RequestCount: 4},
	} {
		require.NoError(t, UpsertPerfMetric(metric))
	}

	metrics, err := GetPerfMetricsByRange("gpt-4o", "alpha", 600, 900)
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	assert.EqualValues(t, 600, metrics[0].BucketTs)
	assert.EqualValues(t, 2, metrics[0].RequestCount)
	assert.EqualValues(t, 900, metrics[1].BucketTs)
	assert.EqualValues(t, 3, metrics[1].RequestCount)
}

func TestGetPerfMetricSummaryBucketsByRange(t *testing.T) {
	preparePerfMetricTable(t)

	for _, metric := range []*PerfMetric{
		{
			ModelName:      "gpt-5.4",
			Group:          "default",
			BucketTs:       300,
			RequestCount:   1,
			SuccessCount:   1,
			TotalLatencyMs: 300,
			OutputTokens:   20,
			GenerationMs:   1000,
		},
		{
			ModelName:      "gpt-5.4",
			Group:          "vip",
			BucketTs:       300,
			RequestCount:   2,
			SuccessCount:   1,
			TotalLatencyMs: 600,
			OutputTokens:   40,
			GenerationMs:   2000,
		},
		{
			ModelName:      "gpt-5.4",
			Group:          "disabled",
			BucketTs:       300,
			RequestCount:   9,
			SuccessCount:   9,
			TotalLatencyMs: 900,
			OutputTokens:   90,
			GenerationMs:   9000,
		},
		{
			ModelName:      "gpt-5.4-mini",
			Group:          "default",
			BucketTs:       600,
			RequestCount:   4,
			SuccessCount:   3,
			TotalLatencyMs: 1200,
			OutputTokens:   80,
			GenerationMs:   4000,
		},
		{
			ModelName:      "gpt-5.4",
			Group:          "default",
			BucketTs:       299,
			RequestCount:   7,
			SuccessCount:   6,
			TotalLatencyMs: 700,
			OutputTokens:   70,
			GenerationMs:   7000,
		},
		{
			ModelName:      "gpt-zero",
			Group:          "default",
			BucketTs:       600,
			RequestCount:   0,
			SuccessCount:   1,
			TotalLatencyMs: 10,
			OutputTokens:   1,
			GenerationMs:   10,
		},
	} {
		require.NoError(t, UpsertPerfMetric(metric))
	}

	summaries, err := GetPerfMetricSummaryBucketsByRange(300, 600, []string{"default", "vip"})
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	assert.Equal(t, PerfMetricSummaryBucket{
		ModelName:      "gpt-5.4",
		BucketTs:       300,
		RequestCount:   3,
		SuccessCount:   2,
		TotalLatencyMs: 900,
		OutputTokens:   60,
		GenerationMs:   3000,
	}, summaries[0])

	assert.Equal(t, PerfMetricSummaryBucket{
		ModelName:      "gpt-5.4-mini",
		BucketTs:       600,
		RequestCount:   4,
		SuccessCount:   3,
		TotalLatencyMs: 1200,
		OutputTokens:   80,
		GenerationMs:   4000,
	}, summaries[1])
}

func TestGetPerfMetricSummaryBucketsByRangeEmptyGroups(t *testing.T) {
	preparePerfMetricTable(t)

	previousDB := DB
	DB = nil
	t.Cleanup(func() {
		DB = previousDB
	})

	summaries, err := GetPerfMetricSummaryBucketsByRange(300, 600, []string{})
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestDeleteExpiredPerfMetricsBoundary(t *testing.T) {
	preparePerfMetricTable(t)

	for _, metric := range []*PerfMetric{
		{ModelName: "gpt-4o", Group: "alpha", BucketTs: 300, RequestCount: 1},
		{ModelName: "gpt-4o", Group: "alpha", BucketTs: 600, RequestCount: 1},
		{ModelName: "gpt-4o", Group: "alpha", BucketTs: 900, RequestCount: 1},
	} {
		require.NoError(t, UpsertPerfMetric(metric))
	}

	rowsAffected, err := DeleteExpiredPerfMetrics(600)
	require.NoError(t, err)
	assert.EqualValues(t, 1, rowsAffected)

	var metrics []PerfMetric
	require.NoError(t, DB.Order("bucket_ts ASC").Find(&metrics).Error)
	require.Len(t, metrics, 2)
	assert.EqualValues(t, 600, metrics[0].BucketTs)
	assert.EqualValues(t, 900, metrics[1].BucketTs)
}
