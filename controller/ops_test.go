package controller

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	opsmetrics "github.com/QuantumNous/new-api/pkg/ops_metrics"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performOpsRequest(t *testing.T, target string, handler gin.HandlerFunc) map[string]any {
	t.Helper()
	recorder := performMonitorGroupRequest(t, http.MethodGet, target, "", handler)
	response := decodeMonitorGroupResponse(t, recorder)
	require.True(t, response["success"].(bool))
	return response
}

func TestOpsOverviewAndRankingsUseAggregatedMetrics(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Name: "Primary", Key: "secret", Type: 1}
	require.NoError(t, model.DB.Create(channel).Error)
	now := time.Now().Unix()
	bucket := now - now%60
	require.NoError(t, model.UpsertOpsMetrics(model.OpsMetricBucket{
		BucketTs: bucket, ModelName: "gpt-5.4", Group: "default", ChannelId: channel.Id, ChannelType: channel.Type,
		RequestCount: 4, SuccessCount: 3, UpstreamErrorCount: 1, TotalLatencyMs: 800,
		TtftSumMs: 120, TtftCount: 3, OutputTokens: 80, GenerationMs: 400,
	}, []model.OpsMetricHistogram{{
		BucketTs: bucket, Metric: "duration", Group: "default", ChannelId: channel.Id, ChannelType: channel.Type, UpperBoundMs: 250, Count: 4,
	}}))

	query := "?start_timestamp=" + stringInt(bucket-60) + "&end_timestamp=" + stringInt(bucket+60)
	overview := performOpsRequest(t, "/api/ops/overview"+query, GetOpsOverview)
	data := overview["data"].(map[string]any)
	assert.EqualValues(t, 4, data["request_count"])
	assert.EqualValues(t, 80, data["token_count"])
	assert.Equal(t, 75.0, data["sla"])
	assert.Equal(t, 25.0, data["upstream_error_rate"])

	rankings := performOpsRequest(t, "/api/ops/rankings"+query, GetOpsRankings)
	rows := rankings["data"].([]any)
	require.Len(t, rows, 1)
	row := rows[0].(map[string]any)
	assert.Equal(t, "Primary", row["channel_name"])
	assert.Equal(t, "gpt-5.4", row["model_name"])
}

func TestBuildOpsOverviewUsesRealMetricsAndStructuredAlerts(t *testing.T) {
	prepareMonitorRunnerTables(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SystemEventLog{}))

	bucket := time.Now().Unix() / 60 * 60
	result := opsmetrics.MetricQueryResult{
		Buckets: []model.OpsMetricBucket{
			{
				BucketTs: bucket, ModelName: "gpt-5.4", Group: "default", ChannelId: 1, ChannelType: 1,
				RequestCount: 5, SuccessCount: 3, BusinessLimitedCount: 1,
				UpstreamErrorCount: 1, Upstream429Count: 1,
				TotalLatencyMs: 600, TtftSumMs: 150, TtftCount: 3,
				OutputTokens: 80, GenerationMs: 400,
			},
			{
				BucketTs: bucket + 60, ModelName: "gpt-5.4", Group: "default", ChannelId: 1, ChannelType: 1,
				RequestCount: 4, SuccessCount: 4,
				TotalLatencyMs: 400, TtftSumMs: 80, TtftCount: 2,
				OutputTokens: 100, GenerationMs: 1000,
			},
		},
		Histograms: []model.OpsMetricHistogram{
			{BucketTs: bucket, Metric: "duration", Group: "default", ChannelId: 1, ChannelType: 1, UpperBoundMs: 250, Count: 5},
			{BucketTs: bucket, Metric: "duration", Group: "default", ChannelId: 1, ChannelType: 1, UpperBoundMs: 500, Count: 4},
			{BucketTs: bucket, Metric: "ttft", Group: "default", ChannelId: 1, ChannelType: 1, UpperBoundMs: 100, Count: 3},
			{BucketTs: bucket, Metric: "ttft", Group: "default", ChannelId: 1, ChannelType: 1, UpperBoundMs: 200, Count: 2},
		},
	}

	rows := make([]model.SystemEventLog, 0, 12)
	for index := 0; index < 12; index++ {
		level := "warn"
		if index%2 == 1 {
			level = "error"
		}
		if index == 4 {
			level = "info"
		}
		rows = append(rows, model.SystemEventLog{
			CreatedAt: bucket + 1000 + int64(index),
			Level:     level,
			Component: "ops_metrics",
			Message:   fmt.Sprintf("event-%02d", index),
		})
	}
	require.NoError(t, model.InsertSystemEventLogs(rows))

	overview := buildOpsOverview(result)

	assert.EqualValues(t, 9, overview.RequestCount)
	assert.EqualValues(t, 7, overview.SuccessCount)
	assert.EqualValues(t, 1, overview.ErrorCount)
	assert.EqualValues(t, 1, overview.BusinessLimitedCount)
	assert.EqualValues(t, 8, overview.SLASampleCount)
	assert.Equal(t, 87.5, overview.SLA)
	assert.Equal(t, 11.11, overview.ErrorRate)
	assert.Equal(t, 12.5, overview.UpstreamRate)

	assert.InDelta(t, 4.0/60.0, overview.QPS.Current, 1e-9)
	assert.InDelta(t, 5.0/60.0, overview.QPS.Peak, 1e-9)
	assert.InDelta(t, 4.5/60.0, overview.QPS.Average, 1e-9)
	assert.Equal(t, 100.0, overview.TPS.Current)
	assert.Equal(t, 200.0, overview.TPS.Peak)
	assert.Equal(t, 150.0, overview.TPS.Average)

	require.NotNil(t, overview.TTFT.P50Ms)
	require.NotNil(t, overview.Duration.P90Ms)
	require.NotNil(t, overview.Duration.P95Ms)
	require.NotNil(t, overview.Duration.MaxMs)
	assert.EqualValues(t, 500, *overview.Duration.MaxMs)

	require.Len(t, overview.RecentAlerts, 10)
	assert.Equal(t, "event-11", overview.RecentAlerts[0].Message)
	assert.Equal(t, "event-01", overview.RecentAlerts[len(overview.RecentAlerts)-1].Message)
	for _, alert := range overview.RecentAlerts {
		assert.NotEqual(t, "info", alert.Level)
	}
}

func TestOpsPercentilesReturnNilWithoutSamples(t *testing.T) {
	percentiles := opsPercentiles(nil, "duration", 0, 0)
	assert.EqualValues(t, 0, percentiles.AverageMs)
	assert.Nil(t, percentiles.P50Ms)
	assert.Nil(t, percentiles.P90Ms)
	assert.Nil(t, percentiles.P95Ms)
	assert.Nil(t, percentiles.P99Ms)
	assert.Nil(t, percentiles.MaxMs)
}

func TestOpsRequestHealthPenaltyIgnoresEmptySamples(t *testing.T) {
	assert.Zero(t, opsRequestHealthPenalty(dto.OpsOverview{}))
	assert.Zero(t, opsRequestHealthPenalty(dto.OpsOverview{
		RequestCount:   3,
		SLASampleCount: 0,
		SLA:            0,
	}))

	assert.Equal(t, 25.0, opsRequestHealthPenalty(dto.OpsOverview{
		RequestCount:   1,
		SLASampleCount: 1,
		SLA:            0,
	}))
}

func TestBuildOpsBackgroundTaskSummaryClassifiesHeartbeats(t *testing.T) {
	now := time.Now()
	summary := buildOpsBackgroundTaskSummary([]common.JobHeartbeat{
		{Name: "monitor_group_runner", Status: "ok", UpdatedAt: now.Unix()},
		{Name: "ops_metrics_flush", Status: "error", UpdatedAt: now.Unix()},
		{Name: "perf_metrics_flush", Status: "ok", UpdatedAt: now.Add(-time.Hour).Unix()},
	}, now)

	assert.Equal(t, len(opsJobHeartbeatMaxAges), summary.Total)
	assert.Equal(t, 1, summary.Healthy)
	assert.Equal(t, 1, summary.Error)
	assert.Equal(t, len(opsJobHeartbeatMaxAges)-2, summary.Stale)
}

func TestGetOpsSystemIncludesGoroutinesAndWriterStats(t *testing.T) {
	prepareMonitorRunnerTables(t)
	common.MarkJobHeartbeat("monitor_group_runner", "ok", "")
	common.MarkJobHeartbeat("ops_metrics_flush", "error", "flush failed")

	response := performOpsRequest(t, "/api/ops/system?_seed="+strconv.Itoa(rand.Int()), GetOpsSystem)
	data := response["data"].(map[string]any)

	assert.GreaterOrEqual(t, int(data["goroutines"].(float64)), 1)
	backgroundTasks := data["background_tasks"].(map[string]any)
	assert.EqualValues(t, len(opsJobHeartbeatMaxAges), backgroundTasks["total"])

	writerStats := data["system_event_writer"].(map[string]any)
	assert.EqualValues(t, 0, writerStats["pending_count"])
	assert.EqualValues(t, 0, writerStats["capacity"])
}

func TestOpsQueryRejectsInvalidRange(t *testing.T) {
	prepareMonitorRunnerTables(t)
	recorder := performMonitorGroupRequest(t, http.MethodGet, "/api/ops/overview?start_timestamp=200&end_timestamp=100", "", GetOpsOverview)
	response := decodeMonitorGroupResponse(t, recorder)
	assert.False(t, response["success"].(bool))
}

func stringInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
