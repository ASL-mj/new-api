package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestRecordTextQuotaMetricsUsesCompletionTokens(t *testing.T) {
	original := recordRelayQuotaSample
	t.Cleanup(func() {
		recordRelayQuotaSample = original
	})

	type relaySample struct {
		info         *relaycommon.RelayInfo
		success      bool
		outputTokens int64
	}

	recorded := make(chan relaySample, 1)
	recordRelayQuotaSample = func(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
		recorded <- relaySample{info: info, success: success, outputTokens: outputTokens}
	}

	relayInfo := &relaycommon.RelayInfo{OriginModelName: "gpt-4.1", UsingGroup: "default"}
	recordTextQuotaMetrics(relayInfo, textQuotaSummary{CompletionTokens: 37})

	select {
	case sample := <-recorded:
		require.Same(t, relayInfo, sample.info)
		require.True(t, sample.success)
		require.EqualValues(t, 37, sample.outputTokens)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for text quota metric")
	}
}

func TestRecordAudioQuotaMetricsUsesCompletionTokens(t *testing.T) {
	original := recordRelayQuotaSample
	t.Cleanup(func() {
		recordRelayQuotaSample = original
	})

	type relaySample struct {
		info         *relaycommon.RelayInfo
		success      bool
		outputTokens int64
	}

	recorded := make(chan relaySample, 1)
	recordRelayQuotaSample = func(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
		recorded <- relaySample{info: info, success: success, outputTokens: outputTokens}
	}

	relayInfo := &relaycommon.RelayInfo{OriginModelName: "whisper-1", UsingGroup: "default"}
	recordAudioQuotaMetrics(relayInfo, &dto.Usage{CompletionTokens: 19})

	select {
	case sample := <-recorded:
		require.Same(t, relayInfo, sample.info)
		require.True(t, sample.success)
		require.EqualValues(t, 19, sample.outputTokens)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for audio quota metric")
	}
}

func TestRecordRealtimeQuotaMetricsUsesOutputTokens(t *testing.T) {
	original := recordRelayQuotaSample
	t.Cleanup(func() {
		recordRelayQuotaSample = original
	})

	type relaySample struct {
		info         *relaycommon.RelayInfo
		success      bool
		outputTokens int64
	}

	recorded := make(chan relaySample, 1)
	recordRelayQuotaSample = func(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
		recorded <- relaySample{info: info, success: success, outputTokens: outputTokens}
	}

	relayInfo := &relaycommon.RelayInfo{OriginModelName: "gpt-realtime", UsingGroup: "default"}
	recordRealtimeQuotaMetrics(relayInfo, &dto.RealtimeUsage{OutputTokens: 23})

	select {
	case sample := <-recorded:
		require.Same(t, relayInfo, sample.info)
		require.True(t, sample.success)
		require.EqualValues(t, 23, sample.outputTokens)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime quota metric")
	}
}
