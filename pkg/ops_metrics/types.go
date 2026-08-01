package opsmetrics

import "time"

type Sample struct {
	BucketTime   time.Time
	Model        string
	Group        string
	ChannelId    int
	ChannelType  int
	Success      bool
	StatusCode   int
	ErrorCode    string
	LocalError   bool
	LatencyMs    int64
	TtftMs       int64
	HasTtft      bool
	OutputTokens int64
	GenerationMs int64
}

type ErrorClass string

const (
	ErrorClassNone            ErrorClass = "none"
	ErrorClassBusinessLimited ErrorClass = "business_limited"
	ErrorClassUpstream        ErrorClass = "upstream"
	ErrorClassSystem          ErrorClass = "system"
)
