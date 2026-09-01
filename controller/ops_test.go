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
	"github.com/QuantumNous/new-api/setting/monitoring_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
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
	previousNow := opsNowFunc
	opsNowFunc = func() time.Time { return time.Unix(bucket+70, 0) }
	t.Cleanup(func() { opsNowFunc = previousNow })
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

	assert.InDelta(t, 4.0/11.0, overview.QPS.Current, 1e-9)
	assert.InDelta(t, 4.0/11.0, overview.QPS.Peak, 1e-9)
	assert.InDelta(t, (5.0/60.0+4.0/11.0)/2, overview.QPS.Average, 1e-9)
	assert.InDelta(t, 100.0/11.0, overview.TPS.Current, 0.01)
	assert.InDelta(t, 100.0/11.0, overview.TPS.Peak, 0.01)
	assert.InDelta(t, (80.0/60.0+100.0/11.0)/2, overview.TPS.Average, 0.01)

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

func TestOpsBucketDurationUsesFullHistoricalAndElapsedCurrentMinute(t *testing.T) {
	now := time.Unix(10*60+14, 0)
	assert.Equal(t, 60.0, opsBucketDurationSeconds(8*60, now))
	assert.Equal(t, 15.0, opsBucketDurationSeconds(10*60, now))
	assert.Equal(t, 1.0, opsBucketDurationSeconds(11*60, now))
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
	previousMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	t.Setenv("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", "true")
	now := time.Now()
	summary := buildOpsBackgroundTaskSummary([]common.JobHeartbeat{
		{Name: "monitor_group_runner", Status: "ok", UpdatedAt: now.Unix()},
		{Name: "ops_metrics_flush", Status: "error", UpdatedAt: now.Unix()},
		{Name: "perf_metrics_flush", Status: "ok", UpdatedAt: now.Add(-time.Hour).Unix()},
	}, now)

	assert.Equal(t, len(expectedOpsJobHeartbeatMaxAges()), summary.Total)
	assert.Equal(t, 1, summary.Healthy)
	assert.Equal(t, 1, summary.Error)
	assert.Equal(t, len(expectedOpsJobHeartbeatMaxAges())-2, summary.Stale)
}

func TestExpectedOpsJobsExcludeMasterOnlyAndDisabledCollectors(t *testing.T) {
	previousMaster := common.IsMasterNode
	previousMonitoring := *monitoring_setting.GetMonitoringSetting()
	previousPerf := *perf_metrics_setting.GetPerfMetricsSetting()
	t.Cleanup(func() {
		common.IsMasterNode = previousMaster
		*monitoring_setting.GetMonitoringSetting() = previousMonitoring
		*perf_metrics_setting.GetPerfMetricsSetting() = previousPerf
	})

	common.IsMasterNode = false
	monitoring_setting.GetMonitoringSetting().OpsEnabled = false
	perf_metrics_setting.GetPerfMetricsSetting().Enabled = true

	expected := expectedOpsJobHeartbeatMaxAges()
	assert.Equal(t, map[string]time.Duration{
		"perf_metrics_flush": opsJobHeartbeatMaxAges["perf_metrics_flush"],
	}, expected)
}

func TestGetOpsSystemIncludesGoroutinesAndWriterStats(t *testing.T) {
	prepareMonitorRunnerTables(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	t.Setenv("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", "true")
	common.MarkJobHeartbeat("monitor_group_runner", "ok", "")
	common.MarkJobHeartbeat("ops_metrics_flush", "error", "flush failed")

	response := performOpsRequest(t, "/api/ops/system?_seed="+strconv.Itoa(rand.Int()), GetOpsSystem)
	data := response["data"].(map[string]any)

	assert.GreaterOrEqual(t, int(data["goroutines"].(float64)), 1)
	backgroundTasks := data["background_tasks"].(map[string]any)
	assert.EqualValues(t, len(expectedOpsJobHeartbeatMaxAges()), backgroundTasks["total"])

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

func TestOpsQueryErrorsAreLocalized(t *testing.T) {
	prepareMonitorRunnerTables(t)
	english := performMonitorGroupRequestWithLanguage(
		t, http.MethodGet, "/api/ops/overview?start_timestamp=200&end_timestamp=100", "", "en", GetOpsOverview,
	)
	assert.Contains(t, english.Body.String(), "The end timestamp cannot be earlier than the start timestamp")

	traditional := performMonitorGroupRequestWithLanguage(
		t, http.MethodGet, "/api/ops/details?metric=invalid", "", "zh-TW", GetOpsDetails,
	)
	assert.Contains(t, traditional.Body.String(), "維運指標無效")
}

func TestGetOpsDetailsPaginatesNewestFirstAndIncludesChannelName(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Name: "Detail Channel", Key: "secret", Type: 1}
	require.NoError(t, model.DB.Create(channel).Error)
	for _, createdAt := range []int64{120, 180} {
		require.NoError(t, model.LOG_DB.Create(&model.Log{
			CreatedAt: createdAt, Type: model.LogTypeConsume, ModelName: "gpt-5.4", Group: "default",
			ChannelId: channel.Id, RequestId: "request-" + stringInt(createdAt), UseTime: 1,
		}).Error)
	}

	response := performOpsRequest(
		t,
		"/api/ops/details?metric=requests&start_timestamp=60&end_timestamp=240&p=1&page_size=1",
		GetOpsDetails,
	)
	data := response["data"].(map[string]any)
	page := data["page"].(map[string]any)
	assert.EqualValues(t, 2, page["total"])
	items := page["items"].([]any)
	require.Len(t, items, 1)
	row := items[0].(map[string]any)
	assert.EqualValues(t, 180, row["created_at"])
	assert.Equal(t, "request-180", row["request_id"])
	assert.Equal(t, "Detail Channel", row["channel_name"])

	recorder := performMonitorGroupRequest(
		t,
		http.MethodGet,
		"/api/ops/details?metric=invalid&start_timestamp=60&end_timestamp=240",
		"",
		GetOpsDetails,
	)
	invalid := decodeMonitorGroupResponse(t, recorder)
	assert.False(t, invalid["success"].(bool))
}

func TestGetOpsDetailsRequestIdIgnoresDashboardTimeWindow(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Name: "Linked Alert Channel", Key: "secret", Type: 1}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		CreatedAt: 100, Type: model.LogTypeError, ModelName: "gpt-5.4", Group: "default",
		ChannelId: channel.Id, RequestId: "alert-linked-request", Content: "upstream failed",
		Other: common.MapToJsonStr(map[string]interface{}{"status_code": 502, "error_code": "do_request_failed"}),
	}).Error)

	response := performOpsRequest(
		t,
		"/api/ops/details?metric=requests&request_id=alert-linked-request&start_timestamp=200&end_timestamp=240",
		GetOpsDetails,
	)
	page := response["data"].(map[string]any)["page"].(map[string]any)
	assert.EqualValues(t, 1, page["total"])
	row := page["items"].([]any)[0].(map[string]any)
	assert.Equal(t, "alert-linked-request", row["request_id"])
	assert.Equal(t, "upstream", row["error_class"])
}

func TestGetOpsDetailsAppliesMetricFiltersToRawRequests(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Name: "Metrics Channel", Key: "secret", Type: 1}
	require.NoError(t, model.DB.Create(channel).Error)
	logs := []*model.Log{
		{CreatedAt: 100, Type: model.LogTypeConsume, ModelName: "gpt-5.4", Group: "default", ChannelId: channel.Id, RequestId: "success", UseTime: 1, Other: common.MapToJsonStr(map[string]interface{}{"frt": 100})},
		{CreatedAt: 110, Type: model.LogTypeConsume, ModelName: "gpt-5.4", Group: "default", ChannelId: channel.Id, RequestId: "slow", UseTime: 9, Other: common.MapToJsonStr(map[string]interface{}{"frt": 900})},
		{CreatedAt: 120, Type: model.LogTypeError, ModelName: "gpt-5.4", Group: "default", ChannelId: channel.Id, RequestId: "limited", Content: "quota exhausted", Other: common.MapToJsonStr(map[string]interface{}{"status_code": 403, "error_code": "insufficient_user_quota"})},
		{CreatedAt: 130, Type: model.LogTypeError, ModelName: "gpt-5.4", Group: "default", ChannelId: channel.Id, RequestId: "upstream", Content: "upstream failed", Other: common.MapToJsonStr(map[string]interface{}{"status_code": 502, "error_code": "do_request_failed"})},
	}
	for _, log := range logs {
		require.NoError(t, model.LOG_DB.Create(log).Error)
	}

	assertMetricTotal := func(metric string, expected int) {
		t.Helper()
		response := performOpsRequest(t, "/api/ops/details?metric="+metric+"&start_timestamp=60&end_timestamp=240", GetOpsDetails)
		page := response["data"].(map[string]any)["page"].(map[string]any)
		assert.EqualValues(t, expected, page["total"], metric)
	}
	assertMetricTotal("requests", 4)
	assertMetricTotal("sla", 3)
	assertMetricTotal("errors", 2)
	assertMetricTotal("upstream", 1)
	assertMetricTotal("duration", 1)
	assertMetricTotal("ttft", 1)
}

func TestGetOpsTrendsReturnsChronologicalWallClockRates(t *testing.T) {
	prepareMonitorRunnerTables(t)
	previousNow := opsNowFunc
	opsNowFunc = func() time.Time { return time.Unix(190, 0) }
	t.Cleanup(func() { opsNowFunc = previousNow })
	for _, bucket := range []model.OpsMetricBucket{
		{BucketTs: 180, ModelName: "gpt-5.4", Group: "default", RequestCount: 11, OutputTokens: 22},
		{BucketTs: 120, ModelName: "gpt-5.4", Group: "default", RequestCount: 60, OutputTokens: 120},
	} {
		require.NoError(t, model.UpsertOpsMetrics(bucket, nil))
	}

	response := performOpsRequest(
		t,
		"/api/ops/trends?start_timestamp=60&end_timestamp=240",
		GetOpsTrends,
	)
	points := response["data"].(map[string]any)["points"].([]any)
	require.Len(t, points, 2)
	first := points[0].(map[string]any)
	second := points[1].(map[string]any)
	assert.EqualValues(t, 120, first["ts"])
	assert.Equal(t, 1.0, first["qps"])
	assert.EqualValues(t, 180, second["ts"])
	assert.Equal(t, 1.0, second["qps"])
	assert.Equal(t, 2.0, second["tps"])
}

func stringInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
