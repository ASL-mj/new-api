package perfmetrics

import "sync"

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

type hotBucket struct {
	mu     sync.Mutex
	data   counters
	closed bool
}

func newHotBucket() *hotBucket {
	return &hotBucket{}
}

func newHotBucketWithCounters(value counters) *hotBucket {
	return &hotBucket{data: value}
}

func (b *hotBucket) add(sample Sample) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}

	b.data.requestCount++
	if sample.Success {
		b.data.successCount++
	}
	b.data.totalLatencyMs += sample.LatencyMs
	if sample.HasTtft {
		b.data.ttftSumMs += sample.TtftMs
		b.data.ttftCount++
	}
	if sample.OutputTokens > 0 {
		b.data.outputTokens += sample.OutputTokens
	}
	if sample.GenerationMs > 0 {
		b.data.generationMs += sample.GenerationMs
	}
	return true
}

func (b *hotBucket) snapshot() counters {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data
}

func (b *hotBucket) closeAndDrain() counters {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	drained := b.data
	b.data = counters{}
	return drained
}

func (b *hotBucket) addCounters(value counters) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	b.data.add(value)
	return true
}
