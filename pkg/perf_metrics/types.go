package perfmetrics

import "sync/atomic"

type Sample struct {
	Model        string
	Group        string
	LatencyMs    int64
	TtftMs       int64
	HasTtft      bool
	Success      bool
	OutputTokens int64
	GenerationMs int64
}

type QueryParams struct {
	Model         string
	Group         string
	Hours         int
	AllowedGroups []string
}

type BucketPoint struct {
	Ts           int64   `json:"ts"`
	RequestCount int64   `json:"request_count"`
	AvgTtftMs    int64   `json:"avg_ttft_ms"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTps       float64 `json:"avg_tps"`
}

type AggregateResult struct {
	RequestCount int64         `json:"request_count"`
	AvgTtftMs    int64         `json:"avg_ttft_ms"`
	AvgLatencyMs int64         `json:"avg_latency_ms"`
	SuccessRate  float64       `json:"success_rate"`
	AvgTps       float64       `json:"avg_tps"`
	Series       []BucketPoint `json:"series"`
}

type GroupResult struct {
	Group string `json:"group"`
	AggregateResult
}

type QueryResult struct {
	ModelName    string          `json:"model_name"`
	Hours        int             `json:"hours"`
	SeriesSchema string          `json:"series_schema"`
	Overall      AggregateResult `json:"overall"`
	Groups       []GroupResult   `json:"groups"`
}

type bucketKey struct {
	model    string
	group    string
	bucketTs int64
}

type counters struct {
	requestCount   int64
	successCount   int64
	totalLatencyMs int64
	ttftSumMs      int64
	ttftCount      int64
	outputTokens   int64
	generationMs   int64
}

type atomicBucket struct {
	requestCount   atomic.Int64
	successCount   atomic.Int64
	totalLatencyMs atomic.Int64
	ttftSumMs      atomic.Int64
	ttftCount      atomic.Int64
	outputTokens   atomic.Int64
	generationMs   atomic.Int64
}

func (b *atomicBucket) add(sample Sample) {
	b.requestCount.Add(1)
	if sample.Success {
		b.successCount.Add(1)
	}
	b.totalLatencyMs.Add(sample.LatencyMs)
	if sample.HasTtft {
		b.ttftSumMs.Add(sample.TtftMs)
		b.ttftCount.Add(1)
	}
	if sample.OutputTokens > 0 {
		b.outputTokens.Add(sample.OutputTokens)
	}
	if sample.GenerationMs > 0 {
		b.generationMs.Add(sample.GenerationMs)
	}
}

func (b *atomicBucket) snapshot() counters {
	return counters{
		requestCount:   b.requestCount.Load(),
		successCount:   b.successCount.Load(),
		totalLatencyMs: b.totalLatencyMs.Load(),
		ttftSumMs:      b.ttftSumMs.Load(),
		ttftCount:      b.ttftCount.Load(),
		outputTokens:   b.outputTokens.Load(),
		generationMs:   b.generationMs.Load(),
	}
}

func (b *atomicBucket) drain() counters {
	return counters{
		requestCount:   b.requestCount.Swap(0),
		successCount:   b.successCount.Swap(0),
		totalLatencyMs: b.totalLatencyMs.Swap(0),
		ttftSumMs:      b.ttftSumMs.Swap(0),
		ttftCount:      b.ttftCount.Swap(0),
		outputTokens:   b.outputTokens.Swap(0),
		generationMs:   b.generationMs.Swap(0),
	}
}

func (b *atomicBucket) addCounters(value counters) {
	if value.requestCount != 0 {
		b.requestCount.Add(value.requestCount)
	}
	if value.successCount != 0 {
		b.successCount.Add(value.successCount)
	}
	if value.totalLatencyMs != 0 {
		b.totalLatencyMs.Add(value.totalLatencyMs)
	}
	if value.ttftSumMs != 0 {
		b.ttftSumMs.Add(value.ttftSumMs)
	}
	if value.ttftCount != 0 {
		b.ttftCount.Add(value.ttftCount)
	}
	if value.outputTokens != 0 {
		b.outputTokens.Add(value.outputTokens)
	}
	if value.generationMs != 0 {
		b.generationMs.Add(value.generationMs)
	}
}
