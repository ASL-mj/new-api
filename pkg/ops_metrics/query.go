package opsmetrics

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/model"
)

var (
	getOpsMetricBucketsByRange = model.GetOpsMetricBucketsByRange
	getOpsMetricBuckets        = model.GetOpsMetricBuckets
	getOpsMetricHistograms     = model.GetOpsMetricHistograms
)

type MetricQuery struct {
	StartBucketTs int64
	EndBucketTs   int64
	Group         string
	ChannelType   *int
	ChannelID     *int
	Model         string
}

type MetricQueryResult struct {
	Buckets    []model.OpsMetricBucket
	Histograms []model.OpsMetricHistogram
}

type ChannelSuccessRateQuery struct {
	ChannelIDs []int
	Model      string
}

// QueryMetrics merges persisted buckets with the local hot buckets. Callers
// receive only aggregate records and never individual API requests.
func QueryMetrics(query MetricQuery) (MetricQueryResult, error) {
	query = normalizeMetricQuery(query)
	channelIDs := queryChannelIDs(query.ChannelID)
	modelQuery := model.OpsMetricQuery{
		StartBucketTs: query.StartBucketTs,
		EndBucketTs:   query.EndBucketTs,
		Group:         query.Group,
		ChannelType:   query.ChannelType,
		ChannelIDs:    channelIDs,
		ModelName:     query.Model,
	}

	queryFlushMu.RLock()
	defer queryFlushMu.RUnlock()

	buckets, err := getOpsMetricBuckets(modelQuery)
	if err != nil {
		return MetricQueryResult{}, err
	}
	result := MetricQueryResult{Buckets: buckets, Histograms: make([]model.OpsMetricHistogram, 0)}
	if query.Model == "" {
		result.Histograms, err = getOpsMetricHistograms(modelQuery)
		if err != nil {
			return MetricQueryResult{}, err
		}
	}

	hotBuckets.Range(func(key, value any) bool {
		keyData := key.(bucketKey)
		if !metricQueryMatchesKey(query, keyData) {
			return true
		}
		snapshot, duration, ttft := value.(*hotBucket).snapshotWithHistograms()
		if snapshot.requestCount == 0 {
			return true
		}
		result.Buckets = append(result.Buckets, model.OpsMetricBucket{
			BucketTs:             keyData.bucketTs,
			ModelName:            keyData.model,
			Group:                keyData.group,
			ChannelId:            keyData.channelId,
			ChannelType:          keyData.channelType,
			RequestCount:         snapshot.requestCount,
			SuccessCount:         snapshot.successCount,
			BusinessLimitedCount: snapshot.businessLimitedCount,
			UpstreamErrorCount:   snapshot.upstreamErrorCount,
			Upstream429Count:     snapshot.upstream429Count,
			Upstream529Count:     snapshot.upstream529Count,
			TotalLatencyMs:       snapshot.totalLatencyMs,
			TtftSumMs:            snapshot.ttftSumMs,
			TtftCount:            snapshot.ttftCount,
			OutputTokens:         snapshot.outputTokens,
			GenerationMs:         snapshot.generationMs,
		})
		if query.Model == "" {
			result.Histograms = append(result.Histograms, histogramRows(keyData, drainedBucket{duration: duration, ttft: ttft})...)
		}
		return true
	})
	return result, nil
}

func normalizeMetricQuery(query MetricQuery) MetricQuery {
	if query.EndBucketTs == 0 {
		query.EndBucketTs = alignTimestamp(nowFunc().Unix())
	}
	if query.StartBucketTs == 0 {
		query.StartBucketTs = alignTimestamp(nowFunc().Add(-time.Hour).Unix())
	}
	query.StartBucketTs = alignTimestamp(query.StartBucketTs)
	query.EndBucketTs = alignTimestamp(query.EndBucketTs)
	return query
}

func queryChannelIDs(channelID *int) []int {
	if channelID == nil {
		return nil
	}
	return []int{*channelID}
}

func metricQueryMatchesKey(query MetricQuery, key bucketKey) bool {
	if key.bucketTs < query.StartBucketTs || key.bucketTs > query.EndBucketTs {
		return false
	}
	if query.Group != "" && query.Group != key.group {
		return false
	}
	if query.ChannelType != nil && *query.ChannelType != key.channelType {
		return false
	}
	if query.ChannelID != nil && *query.ChannelID != key.channelId {
		return false
	}
	return query.Model == "" || query.Model == key.model
}

// QueryChannelSuccessRate returns the final-request success rate for the given
// channels. A nil result means there are no real request samples in the window.
func QueryChannelSuccessRate(channelIDs []int, duration time.Duration) (*float64, error) {
	rates, err := QueryChannelSuccessRates(map[int]ChannelSuccessRateQuery{
		0: {ChannelIDs: channelIDs},
	}, duration)
	if err != nil {
		return nil, err
	}
	return rates[0], nil
}

// QueryChannelSuccessRates computes success rates for multiple channel sets
// with one persisted-bucket query, avoiding one database round trip per group.
func QueryChannelSuccessRates(channelGroups map[int]ChannelSuccessRateQuery, duration time.Duration) (map[int]*float64, error) {
	rates := make(map[int]*float64, len(channelGroups))
	channelToGroups := make(map[int][]int)
	modelByGroup := make(map[int]string, len(channelGroups))
	channelSet := make(map[int]struct{})
	for groupID, query := range channelGroups {
		rates[groupID] = nil
		modelByGroup[groupID] = query.Model
		seen := make(map[int]struct{}, len(query.ChannelIDs))
		for _, channelID := range query.ChannelIDs {
			if channelID <= 0 {
				continue
			}
			if _, exists := seen[channelID]; exists {
				continue
			}
			seen[channelID] = struct{}{}
			channelSet[channelID] = struct{}{}
			channelToGroups[channelID] = append(channelToGroups[channelID], groupID)
		}
	}
	if len(channelSet) == 0 {
		return rates, nil
	}
	if duration <= 0 {
		duration = 24 * time.Hour
	}

	normalizedChannelIDs := make([]int, 0, len(channelSet))
	for channelID := range channelSet {
		normalizedChannelIDs = append(normalizedChannelIDs, channelID)
	}

	queryFlushMu.RLock()
	defer queryFlushMu.RUnlock()

	now := nowFunc()
	startBucketTs := alignTimestamp(now.Add(-duration).Unix())
	endBucketTs := alignTimestamp(now.Unix())
	rows, err := getOpsMetricBucketsByRange(startBucketTs, endBucketTs, normalizedChannelIDs)
	if err != nil {
		return nil, err
	}

	totals := make(map[int]counters, len(channelGroups))
	for _, row := range rows {
		for _, groupID := range channelToGroups[row.ChannelId] {
			if model := modelByGroup[groupID]; model != "" && row.ModelName != model {
				continue
			}
			total := totals[groupID]
			total.add(counters{requestCount: row.RequestCount, successCount: row.SuccessCount})
			totals[groupID] = total
		}
	}
	hotBuckets.Range(func(key, value any) bool {
		bucketKey := key.(bucketKey)
		if bucketKey.bucketTs < startBucketTs || bucketKey.bucketTs > endBucketTs {
			return true
		}
		groupIDs := channelToGroups[bucketKey.channelId]
		if len(groupIDs) == 0 {
			return true
		}
		for _, groupID := range groupIDs {
			if model := modelByGroup[groupID]; model != "" && bucketKey.model != model {
				continue
			}
			total := totals[groupID]
			total.add(value.(*hotBucket).snapshot())
			totals[groupID] = total
		}
		return true
	})

	for groupID, total := range totals {
		if total.requestCount == 0 {
			continue
		}
		rate := math.Round(float64(total.successCount)*10000/float64(total.requestCount)) / 100
		rates[groupID] = &rate
	}
	return rates, nil
}
