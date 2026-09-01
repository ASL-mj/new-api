package dto

type OpsRateSummary struct {
	Current float64 `json:"current"`
	Peak    float64 `json:"peak"`
	Average float64 `json:"average"`
}

type OpsUpstreamError struct {
	Total     int64 `json:"total"`
	Status429 int64 `json:"status_429"`
	Status529 int64 `json:"status_529"`
}

type OpsPercentiles struct {
	AverageMs int64  `json:"average_ms"`
	P50Ms     *int64 `json:"p50_ms"`
	P90Ms     *int64 `json:"p90_ms"`
	P95Ms     *int64 `json:"p95_ms"`
	P99Ms     *int64 `json:"p99_ms"`
	MaxMs     *int64 `json:"max_ms"`
}

type OpsRatePoint struct {
	Ts           int64   `json:"ts"`
	RequestCount int64   `json:"request_count"`
	OutputTokens int64   `json:"output_tokens"`
	QPS          float64 `json:"qps"`
	TPS          float64 `json:"tps"`
	SLA          float64 `json:"sla"`
	ErrorRate    float64 `json:"error_rate"`
}

type OpsAlertItem struct {
	Id         int    `json:"id"`
	CreatedAt  int64  `json:"created_at"`
	Level      string `json:"level"`
	Component  string `json:"component"`
	Message    string `json:"message"`
	RequestId  string `json:"request_id"`
	ChannelId  int    `json:"channel_id"`
	ModelName  string `json:"model_name"`
	Group      string `json:"group"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Extra      string `json:"extra"`
}

type OpsOverview struct {
	HealthScore          int              `json:"health_score"`
	QPS                  OpsRateSummary   `json:"qps"`
	TPS                  OpsRateSummary   `json:"tps"`
	RequestCount         int64            `json:"request_count"`
	SuccessCount         int64            `json:"success_count"`
	ErrorCount           int64            `json:"error_count"`
	BusinessLimitedCount int64            `json:"business_limited_count"`
	SLASampleCount       int64            `json:"sla_sample_count"`
	TokenCount           int64            `json:"token_count"`
	SLA                  float64          `json:"sla"`
	ErrorRate            float64          `json:"error_rate"`
	UpstreamRate         float64          `json:"upstream_error_rate"`
	UpstreamErrors       OpsUpstreamError `json:"upstream_errors"`
	TTFT                 OpsPercentiles   `json:"ttft"`
	Duration             OpsPercentiles   `json:"duration"`
	Realtime             []OpsRatePoint   `json:"realtime"`
	RecentAlerts         []OpsAlertItem   `json:"recent_alerts"`
}

type OpsDetailRow struct {
	BucketTs       int64   `json:"bucket_ts"`
	ModelName      string  `json:"model_name"`
	Group          string  `json:"group"`
	ChannelId      int     `json:"channel_id"`
	ChannelName    string  `json:"channel_name"`
	ChannelType    int     `json:"channel_type"`
	RequestCount   int64   `json:"request_count"`
	SuccessCount   int64   `json:"success_count"`
	SLA            float64 `json:"sla"`
	ErrorRate      float64 `json:"error_rate"`
	UpstreamErrors int64   `json:"upstream_errors"`
	AvgTtftMs      int64   `json:"avg_ttft_ms"`
	AvgDurationMs  int64   `json:"avg_duration_ms"`
}

type OpsRequestDetailRow struct {
	Id               int    `json:"id"`
	CreatedAt        int64  `json:"created_at"`
	Type             int    `json:"type"`
	ModelName        string `json:"model_name"`
	Group            string `json:"group"`
	ChannelId        int    `json:"channel_id"`
	ChannelName      string `json:"channel_name"`
	ChannelType      int    `json:"channel_type"`
	StatusCode       int    `json:"status_code"`
	ErrorClass       string `json:"error_class"`
	ErrorCode        string `json:"error_code"`
	ErrorType        string `json:"error_type"`
	ErrorMessage     string `json:"error_message"`
	RequestPath      string `json:"request_path"`
	RequestId        string `json:"request_id"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Quota            int    `json:"quota"`
	TotalLatencyMs   int64  `json:"total_latency_ms"`
	TtftMs           *int64 `json:"ttft_ms"`
	IsStream         bool   `json:"is_stream"`
}

type OpsRankingRow struct {
	ModelName    string  `json:"model_name"`
	Group        string  `json:"group"`
	ChannelId    int     `json:"channel_id"`
	ChannelName  string  `json:"channel_name"`
	RequestCount int64   `json:"request_count"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTtftMs    int64   `json:"avg_ttft_ms"`
	AvgTps       float64 `json:"avg_tps"`
	Status       string  `json:"status"`
}

type OpsSystemResponse struct {
	CPUUsage          float64                   `json:"cpu_usage"`
	MemoryUsage       float64                   `json:"memory_usage"`
	DiskUsage         float64                   `json:"disk_usage"`
	Goroutines        int                       `json:"goroutines"`
	OpenConnections   int                       `json:"open_connections"`
	InUse             int                       `json:"in_use"`
	Idle              int                       `json:"idle"`
	WaitCount         int64                     `json:"wait_count"`
	WaitDurationMs    int64                     `json:"wait_duration_ms"`
	BackgroundTasks   OpsBackgroundTaskSummary  `json:"background_tasks"`
	SystemEventWriter OpsSystemEventWriterStats `json:"system_event_writer"`
	JobHeartbeats     []OpsJobHeartbeat         `json:"job_heartbeats"`
}

type OpsJobHeartbeat struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	UpdatedAt int64  `json:"updated_at"`
}

type OpsBackgroundTaskSummary struct {
	Total   int `json:"total"`
	Healthy int `json:"healthy"`
	Error   int `json:"error"`
	Stale   int `json:"stale"`
}

type OpsSystemEventWriterStats struct {
	QueuedCount      int64 `json:"queued_count"`
	WrittenCount     int64 `json:"written_count"`
	DroppedCount     int64 `json:"dropped_count"`
	WriteFailedCount int64 `json:"write_failed_count"`
	PendingCount     int   `json:"pending_count"`
	Capacity         int   `json:"capacity"`
}
