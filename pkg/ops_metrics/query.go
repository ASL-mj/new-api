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
	channelSet := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID > 0 {
			channelSet[channelID] = struct{}{}
		}
	}
	if len(channelSet) == 0 {
		return nil, nil
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

	var total counters
	for _, row := range rows {
		total.add(counters{
			requestCount: row.RequestCount,
			successCount: row.SuccessCount,
		})
	}
	hotBuckets.Range(func(key, value any) bool {
		bucketKey := key.(bucketKey)
		if bucketKey.bucketTs < startBucketTs || bucketKey.bucketTs > endBucketTs {
			return true
		}
		if _, ok := channelSet[bucketKey.channelId]; !ok {
			return true
		}
		total.add(value.(*hotBucket).snapshot())
		return true
	})

	if total.requestCount == 0 {
		return nil, nil
	}
	rate := math.Round(float64(total.successCount)*10000/float64(total.requestCount)) / 100
	return &rate, nil
}
