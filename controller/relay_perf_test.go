package controller

import (
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestRecordFinalRelayFailureRecordsSingleAsyncFailure(t *testing.T) {
	original := recordRelayPerformanceSample
	t.Cleanup(func() {
		recordRelayPerformanceSample = original
	})

	type relaySample struct {
		info         *relaycommon.RelayInfo
		success      bool
		outputTokens int64
	}

	recorded := make(chan relaySample, 1)
	recordRelayPerformanceSample = func(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
		recorded <- relaySample{info: info, success: success, outputTokens: outputTokens}
	}

	recordFinalRelayFailure(nil)
	select {
	case sample := <-recorded:
		t.Fatalf("unexpected sample recorded for nil relay info: %+v", sample)
	case <-time.After(50 * time.Millisecond):
	}

	relayInfo := &relaycommon.RelayInfo{OriginModelName: "gpt-4o", UsingGroup: "default"}
	recordFinalRelayFailure(relayInfo)

	select {
	case sample := <-recorded:
		require.Same(t, relayInfo, sample.info)
		require.False(t, sample.success)
		require.Zero(t, sample.outputTokens)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for relay failure metric")
	}

	select {
	case sample := <-recorded:
		t.Fatalf("unexpected extra relay failure metric: %+v", sample)
	case <-time.After(50 * time.Millisecond):
	}
}
