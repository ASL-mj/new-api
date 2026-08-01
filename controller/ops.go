package controller

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	opsmetrics "github.com/QuantumNous/new-api/pkg/ops_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const maxOpsQueryRange = 30 * 24 * time.Hour

var opsJobHeartbeatMaxAges = map[string]time.Duration{
	"monitor_group_runner":     5 * time.Minute,
	"perf_metrics_flush":       15 * time.Minute,
	"ops_metrics_flush":        5 * time.Minute,
	"subscription_quota_reset": 5 * time.Minute,
	"codex_credential_refresh": 30 * time.Minute,
	"channel_upstream_update":  2 * time.Hour,
}

type opsAggregate struct {
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

func (a *opsAggregate) add(row model.OpsMetricBucket) {
	a.requestCount += row.RequestCount
	a.successCount += row.SuccessCount
	a.businessLimitedCount += row.BusinessLimitedCount
	a.upstreamErrorCount += row.UpstreamErrorCount
	a.upstream429Count += row.Upstream429Count
	a.upstream529Count += row.Upstream529Count
	a.totalLatencyMs += row.TotalLatencyMs
	a.ttftSumMs += row.TtftSumMs
	a.ttftCount += row.TtftCount
	a.outputTokens += row.OutputTokens
	a.generationMs += row.GenerationMs
}

func GetOpsOverview(c *gin.Context) {
	result, _, err := queryOpsMetrics(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, buildOpsOverview(result))
}

func GetOpsTrends(c *gin.Context) {
	result, _, err := queryOpsMetrics(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"points": buildOpsRatePoints(result.Buckets)})
}

func GetOpsDetails(c *gin.Context) {
	result, _, err := queryOpsMetrics(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	metric := c.DefaultQuery("metric", "requests")
	if !isValidOpsDetailMetric(metric) {
		common.ApiErrorMsg(c, "invalid metric")
		return
	}
	rows := make([]dto.OpsDetailRow, 0, len(result.Buckets))
	for _, bucket := range result.Buckets {
		rows = append(rows, opsDetailRow(bucket))
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].BucketTs == rows[right].BucketTs {
			return rows[left].RequestCount > rows[right].RequestCount
		}
		return rows[left].BucketTs > rows[right].BucketTs
	})
	pageInfo := common.GetPageQuery(c)
	start := pageInfo.GetStartIdx()
	if start > len(rows) {
		start = len(rows)
	}
	end := pageInfo.GetEndIdx()
	if end > len(rows) {
		end = len(rows)
	}
	pageInfo.SetTotal(len(rows))
	pageInfo.SetItems(rows[start:end])
	common.ApiSuccess(c, gin.H{"metric": metric, "page": pageInfo})
}

func GetOpsRankings(c *gin.Context) {
	result, _, err := queryOpsMetrics(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	type rankingKey struct {
		model string
		group string
		id    int
	}
	groups := make(map[rankingKey]opsAggregate)
	channelIDs := make([]int, 0)
	channelSet := make(map[int]struct{})
	for _, bucket := range result.Buckets {
		key := rankingKey{model: bucket.ModelName, group: bucket.Group, id: bucket.ChannelId}
		aggregate := groups[key]
		aggregate.add(bucket)
		groups[key] = aggregate
		if bucket.ChannelId > 0 {
			if _, seen := channelSet[bucket.ChannelId]; !seen {
				channelSet[bucket.ChannelId] = struct{}{}
				channelIDs = append(channelIDs, bucket.ChannelId)
			}
		}
	}
	channelNames := make(map[int]string, len(channelIDs))
	if len(channelIDs) > 0 {
		channels, err := model.GetChannelsByIds(channelIDs)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, channel := range channels {
			channelNames[channel.Id] = channel.Name
		}
	}
	rows := make([]dto.OpsRankingRow, 0, len(groups))
	for key, aggregate := range groups {
		rows = append(rows, dto.OpsRankingRow{
			ModelName:    key.model,
			Group:        key.group,
			ChannelId:    key.id,
			ChannelName:  channelNameForOps(key.id, channelNames),
			RequestCount: aggregate.requestCount,
			SuccessRate:  percentage(aggregate.successCount, aggregate.requestCount),
			AvgTtftMs:    average(aggregate.ttftSumMs, aggregate.ttftCount),
			AvgTps:       tokensPerSecond(aggregate.outputTokens, aggregate.generationMs),
			Status:       opsStatus(aggregate),
		})
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].RequestCount == rows[right].RequestCount {
			return rows[left].ModelName < rows[right].ModelName
		}
		return rows[left].RequestCount > rows[right].RequestCount
	})
	if len(rows) > 50 {
		rows = rows[:50]
	}
	common.ApiSuccess(c, rows)
}

func GetOpsSystem(c *gin.Context) {
	status := common.GetSystemStatus()
	response := dto.OpsSystemResponse{
		CPUUsage:    status.CPUUsage,
		MemoryUsage: status.MemoryUsage,
		DiskUsage:   status.DiskUsage,
		Goroutines:  runtime.NumGoroutine(),
	}
	heartbeats := common.GetJobHeartbeats()
	response.BackgroundTasks = buildOpsBackgroundTaskSummary(heartbeats, time.Now())
	response.JobHeartbeats = make([]dto.OpsJobHeartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		response.JobHeartbeats = append(response.JobHeartbeats, dto.OpsJobHeartbeat{
			Name: heartbeat.Name, Status: heartbeat.Status, Message: heartbeat.Message, UpdatedAt: heartbeat.UpdatedAt,
		})
	}
	writerStats := service.GetSystemEventWriterStats()
	response.SystemEventWriter = dto.OpsSystemEventWriterStats{
		QueuedCount:      writerStats.QueuedCount,
		WrittenCount:     writerStats.WrittenCount,
		DroppedCount:     writerStats.DroppedCount,
		WriteFailedCount: writerStats.WriteFailedCount,
		PendingCount:     writerStats.PendingCount,
		Capacity:         writerStats.Capacity,
	}
	if model.DB != nil {
		if sqlDB, err := model.DB.DB(); err == nil {
			stats := sqlDB.Stats()
			response.OpenConnections = stats.OpenConnections
			response.InUse = stats.InUse
			response.Idle = stats.Idle
			response.WaitCount = int64(stats.WaitCount)
			response.WaitDurationMs = stats.WaitDuration.Milliseconds()
		}
	}
	common.ApiSuccess(c, response)
}

func queryOpsMetrics(c *gin.Context) (opsmetrics.MetricQueryResult, opsmetrics.MetricQuery, error) {
	filter, err := parseOpsMetricQuery(c)
	if err != nil {
		return opsmetrics.MetricQueryResult{}, opsmetrics.MetricQuery{}, err
	}
	result, err := opsmetrics.QueryMetrics(filter)
	return result, filter, err
}

func parseOpsMetricQuery(c *gin.Context) (opsmetrics.MetricQuery, error) {
	now := time.Now().Unix()
	start, err := optionalTimestamp(c.Query("start_timestamp"), now-int64(time.Hour.Seconds()))
	if err != nil {
		return opsmetrics.MetricQuery{}, fmt.Errorf("invalid start_timestamp")
	}
	end, err := optionalTimestamp(c.Query("end_timestamp"), now)
	if err != nil {
		return opsmetrics.MetricQuery{}, fmt.Errorf("invalid end_timestamp")
	}
	if end < start {
		return opsmetrics.MetricQuery{}, fmt.Errorf("end_timestamp must not be before start_timestamp")
	}
	if time.Duration(end-start)*time.Second > maxOpsQueryRange {
		return opsmetrics.MetricQuery{}, fmt.Errorf("time range cannot exceed 30 days")
	}
	filter := opsmetrics.MetricQuery{
		StartBucketTs: start,
		EndBucketTs:   end,
		Group:         strings.TrimSpace(c.Query("group")),
		Model:         strings.TrimSpace(c.Query("model")),
	}
	if filter.ChannelType, err = optionalNonNegativeInt(c.Query("channel_type")); err != nil {
		return opsmetrics.MetricQuery{}, fmt.Errorf("invalid channel_type")
	}
	if filter.ChannelID, err = optionalNonNegativeInt(c.Query("channel_id")); err != nil {
		return opsmetrics.MetricQuery{}, fmt.Errorf("invalid channel_id")
	}
	return filter, nil
}

func optionalTimestamp(value string, fallback int64) (int64, error) {
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func optionalNonNegativeInt(value string) (*int, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return nil, fmt.Errorf("invalid non-negative integer")
	}
	return &parsed, nil
}

func buildOpsOverview(result opsmetrics.MetricQueryResult) dto.OpsOverview {
	total := opsAggregate{}
	for _, bucket := range result.Buckets {
		total.add(bucket)
	}
	points := buildOpsRatePoints(result.Buckets)
	overview := dto.OpsOverview{
		QPS: buildOpsQPSSummary(points),
		TPS: buildOpsTPSSummary(points),

		RequestCount:         total.requestCount,
		SuccessCount:         total.successCount,
		ErrorCount:           opsErrorCount(total),
		BusinessLimitedCount: total.businessLimitedCount,
		SLASampleCount:       slaDenominator(total),
		TokenCount:           total.outputTokens,
		SLA:                  sla(total),
		ErrorRate:            percentage(opsErrorCount(total), total.requestCount),
		UpstreamRate:         percentage(total.upstreamErrorCount, slaDenominator(total)),
		UpstreamErrors: dto.OpsUpstreamError{
			Total: total.upstreamErrorCount, Status429: total.upstream429Count, Status529: total.upstream529Count,
		},
		TTFT:         opsPercentiles(result.Histograms, "ttft", total.ttftSumMs, total.ttftCount),
		Duration:     opsPercentiles(result.Histograms, "duration", total.totalLatencyMs, total.requestCount),
		Realtime:     points,
		RecentAlerts: recentOpsAlerts(10),
	}
	overview.HealthScore = opsHealthScore(overview)
	return overview
}

func buildOpsRatePoints(buckets []model.OpsMetricBucket) []dto.OpsRatePoint {
	byTimestamp := make(map[int64]opsAggregate)
	for _, bucket := range buckets {
		aggregate := byTimestamp[bucket.BucketTs]
		aggregate.add(bucket)
		byTimestamp[bucket.BucketTs] = aggregate
	}
	timestamps := make([]int64, 0, len(byTimestamp))
	for timestamp := range byTimestamp {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(left, right int) bool { return timestamps[left] < timestamps[right] })
	points := make([]dto.OpsRatePoint, 0, len(timestamps))
	for _, timestamp := range timestamps {
		aggregate := byTimestamp[timestamp]
		points = append(points, dto.OpsRatePoint{
			Ts: timestamp, RequestCount: aggregate.requestCount, OutputTokens: aggregate.outputTokens,
			QPS: float64(aggregate.requestCount) / 60,
			TPS: tokensPerSecond(aggregate.outputTokens, aggregate.generationMs),
			SLA: sla(aggregate), ErrorRate: percentage(opsErrorCount(aggregate), aggregate.requestCount),
		})
	}
	return points
}

func opsDetailRow(bucket model.OpsMetricBucket) dto.OpsDetailRow {
	aggregate := opsAggregate{}
	aggregate.add(bucket)
	return dto.OpsDetailRow{
		BucketTs: bucket.BucketTs, ModelName: bucket.ModelName, Group: bucket.Group, ChannelId: bucket.ChannelId, ChannelType: bucket.ChannelType,
		RequestCount: aggregate.requestCount, SuccessCount: aggregate.successCount, SLA: sla(aggregate),
		ErrorRate: percentage(aggregate.requestCount-aggregate.successCount, aggregate.requestCount), UpstreamErrors: aggregate.upstreamErrorCount,
		AvgTtftMs: average(aggregate.ttftSumMs, aggregate.ttftCount), AvgDurationMs: average(aggregate.totalLatencyMs, aggregate.requestCount),
	}
}

func opsPercentiles(rows []model.OpsMetricHistogram, metric string, totalMs, count int64) dto.OpsPercentiles {
	histogram := make(map[int64]int64)
	for _, row := range rows {
		if row.Metric == metric && row.Count > 0 {
			histogram[row.UpperBoundMs] += row.Count
		}
	}
	bounds := make([]int64, 0, len(histogram))
	for bound := range histogram {
		bounds = append(bounds, bound)
	}
	sort.Slice(bounds, func(left, right int) bool { return bounds[left] < bounds[right] })
	buckets := make([]opsmetrics.HistogramBucket, 0, len(bounds))
	for _, bound := range bounds {
		buckets = append(buckets, opsmetrics.HistogramBucket{UpperBoundMs: bound, Count: histogram[bound]})
	}
	var maxMs *int64
	if len(bounds) > 0 {
		maxValue := bounds[len(bounds)-1]
		maxMs = &maxValue
	}
	return dto.OpsPercentiles{
		AverageMs: average(totalMs, count),
		P50Ms:     opsmetrics.PercentileFromHistogram(buckets, 0.50),
		P90Ms:     opsmetrics.PercentileFromHistogram(buckets, 0.90),
		P95Ms:     opsmetrics.PercentileFromHistogram(buckets, 0.95),
		P99Ms:     opsmetrics.PercentileFromHistogram(buckets, 0.99),
		MaxMs:     maxMs,
	}
}

func isValidOpsDetailMetric(metric string) bool {
	return metric == "requests" || metric == "sla" || metric == "errors" || metric == "upstream" || metric == "ttft" || metric == "duration"
}

func buildOpsQPSSummary(points []dto.OpsRatePoint) dto.OpsRateSummary {
	summary := dto.OpsRateSummary{}
	if len(points) > 0 {
		summary.Current = points[len(points)-1].QPS
		for _, point := range points {
			summary.Average += point.QPS
			if point.QPS > summary.Peak {
				summary.Peak = point.QPS
			}
		}
		summary.Average /= float64(len(points))
	}
	return summary
}

func buildOpsTPSSummary(points []dto.OpsRatePoint) dto.OpsRateSummary {
	summary := dto.OpsRateSummary{}
	if len(points) > 0 {
		summary.Current = points[len(points)-1].TPS
		for _, point := range points {
			summary.Average += point.TPS
			if point.TPS > summary.Peak {
				summary.Peak = point.TPS
			}
		}
		summary.Average /= float64(len(points))
	}
	return summary
}

func buildOpsBackgroundTaskSummary(heartbeats []common.JobHeartbeat, now time.Time) dto.OpsBackgroundTaskSummary {
	summary := dto.OpsBackgroundTaskSummary{Total: len(opsJobHeartbeatMaxAges)}
	heartbeatByName := make(map[string]common.JobHeartbeat, len(heartbeats))
	for _, heartbeat := range heartbeats {
		heartbeatByName[heartbeat.Name] = heartbeat
	}
	for jobName, maxAge := range opsJobHeartbeatMaxAges {
		heartbeat, exists := heartbeatByName[jobName]
		if !exists || heartbeat.UpdatedAt == 0 || now.Sub(time.Unix(heartbeat.UpdatedAt, 0)) > maxAge {
			summary.Stale++
			continue
		}
		if strings.EqualFold(heartbeat.Status, "error") {
			summary.Error++
			continue
		}
		summary.Healthy++
	}
	return summary
}

func recentOpsAlerts(limit int) []dto.OpsAlertItem {
	if limit <= 0 || model.DB == nil {
		return []dto.OpsAlertItem{}
	}
	rows := make([]model.SystemEventLog, 0, limit)
	if err := model.DB.Select("created_at", "level", "component", "message").
		Where("level IN ?", []string{"warn", "error"}).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		common.SysError("failed to load recent ops alerts: " + err.Error())
		return []dto.OpsAlertItem{}
	}
	alerts := make([]dto.OpsAlertItem, 0, len(rows))
	for _, row := range rows {
		alerts = append(alerts, dto.OpsAlertItem{
			CreatedAt: row.CreatedAt,
			Level:     row.Level,
			Component: row.Component,
			Message:   row.Message,
		})
	}
	return alerts
}

func opsErrorCount(value opsAggregate) int64 {
	errorCount := value.requestCount - value.successCount - value.businessLimitedCount
	if errorCount < 0 {
		return 0
	}
	return errorCount
}

func slaDenominator(value opsAggregate) int64 { return value.requestCount - value.businessLimitedCount }
func sla(value opsAggregate) float64          { return percentage(value.successCount, slaDenominator(value)) }
func average(sum, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return sum / count
}
func percentage(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return math.Round(float64(numerator)*10000/float64(denominator)) / 100
}
func tokensPerSecond(tokens, generationMs int64) float64 {
	if tokens <= 0 || generationMs <= 0 {
		return 0
	}
	return math.Round(float64(tokens)*100000/float64(generationMs)) / 100
}

func channelNameForOps(channelID int, names map[int]string) string {
	if channelID == 0 {
		return "未分配渠道"
	}
	return names[channelID]
}
func opsStatus(value opsAggregate) string {
	if value.requestCount == 0 {
		return "unknown"
	}
	if value.successCount == 0 {
		return "failed"
	}
	if value.successCount < value.requestCount {
		return "degraded"
	}
	return "operational"
}

func opsHealthScore(overview dto.OpsOverview) int {
	score := 100.0
	score -= opsRequestHealthPenalty(overview)
	status := common.GetSystemStatus()
	config := common.GetPerformanceMonitorConfig()
	if status.CPUUsage >= float64(config.CPUThreshold) {
		score -= 15
	} else if status.CPUUsage >= float64(config.CPUThreshold)*0.8 {
		score -= 7
	}
	if status.MemoryUsage >= float64(config.MemoryThreshold) {
		score -= 10
	} else if status.MemoryUsage >= float64(config.MemoryThreshold)*0.8 {
		score -= 5
	}
	for jobName, maxAge := range opsJobHeartbeatMaxAges {
		if common.IsJobHeartbeatStale(jobName, maxAge) {
			score -= 10
			break
		}
	}
	if score < 0 {
		return 0
	}
	return int(math.Round(score))
}

func opsRequestHealthPenalty(overview dto.OpsOverview) float64 {
	if overview.SLASampleCount <= 0 {
		return 0
	}
	return math.Min(40, overview.UpstreamRate*4) +
		math.Min(25, math.Max(0, 99.9-overview.SLA)*10)
}
