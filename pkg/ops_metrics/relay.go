package opsmetrics

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

// RecordRelaySuccess records one fully settled client request. It intentionally
// derives the final selected channel from RelayInfo instead of request context,
// so retries are represented by their final outcome only.
func RecordRelaySuccess(info *relaycommon.RelayInfo, outputTokens int64) {
	recordRelaySample(info, Sample{
		Success:      true,
		OutputTokens: outputTokens,
	})
}

// RecordRelayFailure records a failure only after the caller has exhausted its
// retry loop. Pre-consume rejections are supported with channel ID 0 so they
// remain visible to operations without being attributed to an upstream.
func RecordRelayFailure(info *relaycommon.RelayInfo, relayErr *types.NewAPIError) {
	if relayErr == nil {
		return
	}
	recordRelaySample(info, Sample{
		StatusCode: relayErr.StatusCode,
		ErrorCode:  string(relayErr.GetErrorCode()),
		LocalError: relayErr.GetErrorType() == types.ErrorTypeNewAPIError,
	})
}

func recordRelaySample(info *relaycommon.RelayInfo, sample Sample) {
	if info == nil || info.OriginModelName == "" {
		return
	}

	now := nowFunc()
	sample.Model = info.OriginModelName
	sample.Group = info.UsingGroup
	if sample.Group == "" {
		sample.Group = info.TokenGroup
	}
	if info.ChannelMeta != nil {
		sample.ChannelId = info.ChannelId
		sample.ChannelType = info.ChannelType
	}
	sample.LatencyMs = now.Sub(info.StartTime).Milliseconds()
	sample.GenerationMs = sample.LatencyMs
	sample.HasTtft = info.IsStream && info.HasSendResponse()
	if sample.HasTtft {
		sample.TtftMs = info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
		sample.GenerationMs = now.Sub(info.FirstResponseTime).Milliseconds()
	}
	if sample.LatencyMs < 0 {
		sample.LatencyMs = 0
	}
	if sample.GenerationMs < 0 {
		sample.GenerationMs = 0
	}
	if sample.TtftMs < 0 {
		sample.TtftMs = 0
	}
	Record(sample)
}
