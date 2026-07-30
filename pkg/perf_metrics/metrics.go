package perfmetrics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

const (
	defaultGroup       = "default"
	defaultHours       = 24
	maxHours           = 168
	hourSeconds  int64 = 3600
	seriesSchema       = "dbcd0a3c01b55203"
)

var (
	hotBuckets sync.Map
	nowFunc    = time.Now
	initOnce   sync.Once

	queryFlushMu sync.RWMutex
	startFlushFn = func() {
		go flushLoop()
	}

	getPerfMetricsByRange              = model.GetPerfMetricsByRange
	getPerfMetricSummaryBucketsByRange = model.GetPerfMetricSummaryBucketsByRange
	upsertPerfMetric                   = model.UpsertPerfMetric
	deleteExpiredPerfMetrics           = model.DeleteExpiredPerfMetrics
)

func Init() {
	initOnce.Do(startFlushFn)
}

func RecordRelaySample(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
	if info == nil {
		return
	}
	now := nowFunc()
	latencyMs := now.Sub(info.StartTime).Milliseconds()
	hasTtft := info.IsStream && info.HasSendResponse()
	ttftMs := int64(0)
	generationMs := latencyMs
	if hasTtft {
		ttftMs = info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
		if ttftMs < 0 {
			ttftMs = 0
		}
		generationMs = now.Sub(info.FirstResponseTime).Milliseconds()
		if generationMs < 0 {
			generationMs = 0
		}
	}

	Record(Sample{
		Model:        info.OriginModelName,
		Group:        info.UsingGroup,
		LatencyMs:    latencyMs,
		TtftMs:       ttftMs,
		HasTtft:      hasTtft,
		Success:      success,
		OutputTokens: outputTokens,
		GenerationMs: generationMs,
	})
}

func Record(sample Sample) {
	if !perf_metrics_setting.GetPerfMetricsSetting().Enabled {
		return
	}
	if sample.Model == "" {
		return
	}
	if sample.Group == "" {
		sample.Group = defaultGroup
	}
	if sample.LatencyMs < 0 {
		sample.LatencyMs = 0
	}
	if sample.HasTtft && sample.TtftMs < 0 {
		sample.TtftMs = 0
	}
	if sample.OutputTokens < 0 {
		sample.OutputTokens = 0
	}
	if sample.GenerationMs < 0 {
		sample.GenerationMs = 0
	}

	key := bucketKey{
		model:    sample.Model,
		group:    sample.Group,
		bucketTs: alignTimestamp(nowFunc().Unix(), int64(perf_metrics_setting.GetBucketSeconds())),
	}
	for {
		actual, _ := hotBuckets.LoadOrStore(key, newHotBucket())
		bucket := actual.(*hotBucket)
		if bucket.add(sample) {
			recordRedis(key, sample)
			return
		}
		hotBuckets.CompareAndDelete(key, actual)
	}
}

func Query(params QueryParams) (QueryResult, error) {
	params.Hours = normalizeHours(params.Hours)
	allowedGroups := allowedGroupSet(params.AllowedGroups)
	if params.Group == "" && params.AllowedGroups != nil && len(params.AllowedGroups) == 0 {
		return emptyQueryResult(params), nil
	}
	if params.Group != "" && !groupAllowed(params.Group, allowedGroups) {
		return emptyQueryResult(params), nil
	}

	queryFlushMu.RLock()
	defer queryFlushMu.RUnlock()

	nowTs := nowFunc().Unix()
	sourceBucketSeconds := int64(perf_metrics_setting.GetBucketSeconds())
	startBucketTs := alignTimestamp(nowTs-int64(params.Hours)*hourSeconds, sourceBucketSeconds)
	endBucketTs := alignTimestamp(nowTs, sourceBucketSeconds)

	rows, err := getPerfMetricsByRange(params.Model, params.Group, startBucketTs, endBucketTs)
	if err != nil {
		return QueryResult{}, err
	}

	raw := map[bucketKey]counters{}
	for _, row := range rows {
		group := row.Group
		if group == "" {
			group = defaultGroup
		}
		if !groupAllowed(group, allowedGroups) {
			continue
		}
		mergeCounters(raw, bucketKey{
			model:    row.ModelName,
			group:    group,
			bucketTs: row.BucketTs,
		}, counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			ttftSumMs:      row.TtftSumMs,
			ttftCount:      row.TtftCount,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		})
	}

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if params.Model != "" && k.model != params.Model {
			return true
		}
		if params.Group != "" && k.group != params.Group {
			return true
		}
		if k.bucketTs < startBucketTs || k.bucketTs > endBucketTs {
			return true
		}
		if !groupAllowed(k.group, allowedGroups) {
			return true
		}
		mergeCounters(raw, k, value.(*hotBucket).snapshot())
		return true
	})

	// Redis only keeps a best-effort active-bucket mirror; single-instance query
	// intentionally merges DB plus local hotBuckets only to avoid double counting.
	return buildQueryResult(params, raw), nil
}

func QuerySummaryAll(hours int, allowedGroups []string) (SummaryAllResult, error) {
	hours = normalizeHours(hours)
	if allowedGroups != nil && len(allowedGroups) == 0 {
		return SummaryAllResult{Models: []ModelSummary{}}, nil
	}

	queryFlushMu.RLock()
	defer queryFlushMu.RUnlock()

	nowTs := nowFunc().Unix()
	sourceBucketSeconds := int64(perf_metrics_setting.GetBucketSeconds())
	startBucketTs := alignTimestamp(nowTs-int64(hours)*hourSeconds, sourceBucketSeconds)
	endBucketTs := alignTimestamp(nowTs, sourceBucketSeconds)
	allowed := allowedGroupSet(allowedGroups)

	rows, err := getPerfMetricSummaryBucketsByRange(startBucketTs, endBucketTs, allowedGroups)
	if err != nil {
		return SummaryAllResult{}, err
	}

	totals := make(map[string]counters)
	modelBuckets := make(map[string]map[int64]counters)
	for _, row := range rows {
		mergeModelSummaryCounters(totals, modelBuckets, row.ModelName, row.BucketTs, counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		})
	}

	hotBuckets.Range(func(key, value any) bool {
		bucket := key.(bucketKey)
		if bucket.bucketTs < startBucketTs || bucket.bucketTs > endBucketTs || !groupAllowed(bucket.group, allowed) {
			return true
		}
		mergeModelSummaryCounters(totals, modelBuckets, bucket.model, bucket.bucketTs, value.(*hotBucket).snapshot())
		return true
	})

	return buildSummaryAllResult(totals, modelBuckets), nil
}

func normalizeHours(hours int) int {
	if hours <= 0 {
		return defaultHours
	}
	if hours > maxHours {
		return maxHours
	}
	return hours
}

func emptyQueryResult(params QueryParams) QueryResult {
	params.Hours = normalizeHours(params.Hours)
	return QueryResult{
		ModelName:    params.Model,
		Hours:        params.Hours,
		SeriesSchema: seriesSchema,
		Overall: AggregateResult{
			Series: []BucketPoint{},
		},
		Groups: []GroupResult{},
	}
}

func buildQueryResult(params QueryParams, raw map[bucketKey]counters) QueryResult {
	rollupSeconds := rollupSecondsForHours(params.Hours)
	groupSeriesCounters := map[string]map[int64]counters{}
	groupTotals := map[string]counters{}
	overallSeriesCounters := map[int64]counters{}
	overallTotal := counters{}

	for key, value := range raw {
		if value.requestCount == 0 {
			continue
		}
		rollupTs := alignTimestamp(key.bucketTs, rollupSeconds)
		if _, ok := groupSeriesCounters[key.group]; !ok {
			groupSeriesCounters[key.group] = map[int64]counters{}
		}
		mergeSeriesCounters(groupSeriesCounters[key.group], rollupTs, value)

		groupTotal := groupTotals[key.group]
		groupTotal.add(value)
		groupTotals[key.group] = groupTotal

		overallTotal.add(value)
		mergeSeriesCounters(overallSeriesCounters, rollupTs, value)
	}

	groupNames := make([]string, 0, len(groupSeriesCounters))
	for group := range groupSeriesCounters {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)

	groups := make([]GroupResult, 0, len(groupNames))
	for _, group := range groupNames {
		groups = append(groups, GroupResult{
			Group:           group,
			AggregateResult: buildAggregateResult(groupTotals[group], groupSeriesCounters[group]),
		})
	}

	return QueryResult{
		ModelName:    params.Model,
		Hours:        params.Hours,
		SeriesSchema: seriesSchema,
		Overall:      buildAggregateResult(overallTotal, overallSeriesCounters),
		Groups:       groups,
	}
}

func buildAggregateResult(total counters, seriesCounters map[int64]counters) AggregateResult {
	return AggregateResult{
		RequestCount: total.requestCount,
		AvgTtftMs:    avg(total.ttftSumMs, total.ttftCount),
		AvgLatencyMs: avg(total.totalLatencyMs, total.requestCount),
		SuccessRate:  round2(successRate(total)),
		AvgTps:       round2(avgTps(total)),
		Series:       buildSeries(seriesCounters),
	}
}

func buildSeries(seriesCounters map[int64]counters) []BucketPoint {
	if len(seriesCounters) == 0 {
		return []BucketPoint{}
	}
	timestamps := make([]int64, 0, len(seriesCounters))
	for ts := range seriesCounters {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})

	series := make([]BucketPoint, 0, len(timestamps))
	for _, ts := range timestamps {
		value := seriesCounters[ts]
		series = append(series, BucketPoint{
			Ts:           ts,
			RequestCount: value.requestCount,
			AvgTtftMs:    avg(value.ttftSumMs, value.ttftCount),
			AvgLatencyMs: avg(value.totalLatencyMs, value.requestCount),
			SuccessRate:  round2(successRate(value)),
			AvgTps:       round2(avgTps(value)),
		})
	}
	return series
}

func rollupSecondsForHours(hours int) int64 {
	switch normalizeHours(hours) {
	case 1:
		return int64(perf_metrics_setting.GetBucketSeconds())
	case 24, 168:
		return hourSeconds
	default:
		return hourSeconds
	}
}

func alignTimestamp(ts int64, seconds int64) int64 {
	if seconds <= 0 {
		return ts
	}
	return ts - (ts % seconds)
}

func allowedGroupSet(groups []string) map[string]struct{} {
	if groups == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		allowed[group] = struct{}{}
	}
	return allowed
}

func groupAllowed(group string, allowed map[string]struct{}) bool {
	if allowed == nil {
		return true
	}
	_, ok := allowed[group]
	return ok
}

func mergeModelSummaryCounters(totals map[string]counters, modelBuckets map[string]map[int64]counters, modelName string, bucketTs int64, value counters) {
	if modelName == "" || value.requestCount == 0 {
		return
	}
	total := totals[modelName]
	total.add(value)
	totals[modelName] = total

	if _, ok := modelBuckets[modelName]; !ok {
		modelBuckets[modelName] = make(map[int64]counters)
	}
	bucket := modelBuckets[modelName][bucketTs]
	bucket.add(value)
	modelBuckets[modelName][bucketTs] = bucket
}

func buildSummaryAllResult(totals map[string]counters, modelBuckets map[string]map[int64]counters) SummaryAllResult {
	models := make([]ModelSummary, 0, len(totals))
	for modelName, total := range totals {
		if total.requestCount == 0 {
			continue
		}
		models = append(models, ModelSummary{
			ModelName:          modelName,
			AvgLatencyMs:       avg(total.totalLatencyMs, total.requestCount),
			SuccessRate:        round2(successRate(total)),
			AvgTps:             round2(avgTps(total)),
			RecentSuccessRates: recentSuccessRates(modelBuckets[modelName], 3),
			RequestCount:       total.requestCount,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].RequestCount == models[j].RequestCount {
			return models[i].ModelName < models[j].ModelName
		}
		return models[i].RequestCount > models[j].RequestCount
	})
	return SummaryAllResult{Models: models}
}

func recentSuccessRates(buckets map[int64]counters, limit int) []float64 {
	if len(buckets) == 0 || limit <= 0 {
		return nil
	}
	timestamps := make([]int64, 0, len(buckets))
	for timestamp := range buckets {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})
	if len(timestamps) > limit {
		timestamps = timestamps[len(timestamps)-limit:]
	}

	rates := make([]float64, 0, len(timestamps))
	for _, timestamp := range timestamps {
		rates = append(rates, round2(successRate(buckets[timestamp])))
	}
	return rates
}

func mergeCounters(merged map[bucketKey]counters, key bucketKey, value counters) {
	if value.requestCount == 0 {
		return
	}
	current := merged[key]
	current.add(value)
	merged[key] = current
}

func mergeSeriesCounters(merged map[int64]counters, ts int64, value counters) {
	if value.requestCount == 0 {
		return
	}
	current := merged[ts]
	current.add(value)
	merged[ts] = current
}

func (c *counters) add(value counters) {
	c.requestCount += value.requestCount
	c.successCount += value.successCount
	c.totalLatencyMs += value.totalLatencyMs
	c.ttftSumMs += value.ttftSumMs
	c.ttftCount += value.ttftCount
	c.outputTokens += value.outputTokens
	c.generationMs += value.generationMs
}

func avg(sum int64, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return sum / count
}

func successRate(value counters) float64 {
	if value.requestCount <= 0 {
		return 0
	}
	return float64(value.successCount) / float64(value.requestCount) * 100
}

func avgTps(value counters) float64 {
	if value.outputTokens <= 0 || value.generationMs <= 0 {
		return 0
	}
	return float64(value.outputTokens) / (float64(value.generationMs) / 1000.0)
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func recordRedis(key bucketKey, sample Sample) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	redisKey := fmt.Sprintf("perf:%s:%s:%d", key.model, key.group, key.bucketTs)
	pipe := common.RDB.TxPipeline()
	pipe.HIncrBy(ctx, redisKey, "req", 1)
	if sample.Success {
		pipe.HIncrBy(ctx, redisKey, "ok", 1)
	}
	if sample.LatencyMs > 0 {
		pipe.HIncrBy(ctx, redisKey, "lat", sample.LatencyMs)
	}
	if sample.HasTtft {
		pipe.HIncrBy(ctx, redisKey, "ttft", sample.TtftMs)
		pipe.HIncrBy(ctx, redisKey, "ttft_n", 1)
	}
	if sample.OutputTokens > 0 {
		pipe.HIncrBy(ctx, redisKey, "out", sample.OutputTokens)
	}
	if sample.GenerationMs > 0 {
		pipe.HIncrBy(ctx, redisKey, "gen_ms", sample.GenerationMs)
	}
	pipe.Expire(ctx, redisKey, time.Hour)
	_, _ = pipe.Exec(ctx)
}
