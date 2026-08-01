package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordFinalRelayFailureRecordsSingleAsyncFailure(t *testing.T) {
	originalPerformance := recordRelayPerformanceSample
	originalOps := recordRelayOpsFailure
	originalEvent := recordRelaySystemEvent
	t.Cleanup(func() {
		recordRelayPerformanceSample = originalPerformance
		recordRelayOpsFailure = originalOps
		recordRelaySystemEvent = originalEvent
	})

	type relaySample struct {
		info         *relaycommon.RelayInfo
		success      bool
		outputTokens int64
	}

	recorded := make(chan relaySample, 1)
	opsRecorded := make(chan *types.NewAPIError, 1)
	recordRelayPerformanceSample = func(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
		recorded <- relaySample{info: info, success: success, outputTokens: outputTokens}
	}
	recordRelayOpsFailure = func(_ *relaycommon.RelayInfo, relayErr *types.NewAPIError) {
		opsRecorded <- relayErr
	}
	recordRelaySystemEvent = func(model.SystemEventLog) {}

	recordFinalRelayFailure(nil, nil)
	select {
	case sample := <-recorded:
		t.Fatalf("unexpected sample recorded for nil relay info: %+v", sample)
	case <-time.After(50 * time.Millisecond):
	}

	relayInfo := &relaycommon.RelayInfo{OriginModelName: "gpt-4o", UsingGroup: "default"}
	relayErr := types.NewError(errors.New("upstream failed"), types.ErrorCodeDoRequestFailed)
	recordFinalRelayFailure(relayInfo, relayErr)

	select {
	case sample := <-recorded:
		require.Same(t, relayInfo, sample.info)
		require.False(t, sample.success)
		require.Zero(t, sample.outputTokens)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for relay failure metric")
	}

	select {
	case got := <-opsRecorded:
		require.Same(t, relayErr, got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for relay ops failure metric")
	}

	select {
	case sample := <-recorded:
		t.Fatalf("unexpected extra relay failure metric: %+v", sample)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRelayValidationFailureDoesNotRecordPerformance(t *testing.T) {
	originalPerformance := recordRelayPerformanceSample
	originalOps := recordRelayOpsFailure
	originalEvent := recordRelaySystemEvent
	t.Cleanup(func() {
		recordRelayPerformanceSample = originalPerformance
		recordRelayOpsFailure = originalOps
		recordRelaySystemEvent = originalEvent
	})

	recorded := make(chan struct{}, 1)
	recordRelayPerformanceSample = func(*relaycommon.RelayInfo, bool, int64) {
		recorded <- struct{}{}
	}
	recordRelayOpsFailure = func(*relaycommon.RelayInfo, *types.NewAPIError) {
		recorded <- struct{}{}
	}
	recordRelaySystemEvent = func(model.SystemEventLog) {
		recorded <- struct{}{}
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{"))
	ctx.Request.Header.Set("Content-Type", "application/json")
	Relay(ctx, types.RelayFormatOpenAI)

	select {
	case <-recorded:
		t.Fatal("validation failure must not record a relay performance sample")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRecordTaskOpsResultRecordsTaskOutcomeAndUpstreamEvent(t *testing.T) {
	originalSuccess := recordTaskOpsSuccess
	originalFailure := recordTaskOpsFailure
	originalEvent := recordRelaySystemEvent
	t.Cleanup(func() {
		recordTaskOpsSuccess = originalSuccess
		recordTaskOpsFailure = originalFailure
		recordRelaySystemEvent = originalEvent
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "task-model", UsingGroup: "default", StartTime: time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 12, ChannelType: 1},
	}
	successes := make(chan *relaycommon.RelayInfo, 1)
	failures := make(chan *types.NewAPIError, 1)
	events := make(chan model.SystemEventLog, 1)
	recordTaskOpsSuccess = func(got *relaycommon.RelayInfo, _ int64) { successes <- got }
	recordTaskOpsFailure = func(_ *relaycommon.RelayInfo, got *types.NewAPIError) { failures <- got }
	recordRelaySystemEvent = func(event model.SystemEventLog) { events <- event }

	recordTaskOpsResult(info, nil)
	require.Same(t, info, <-successes)

	recordTaskOpsResult(info, &dto.TaskError{
		Code: "upstream_task_failed", Error: errors.New("upstream unavailable"), StatusCode: http.StatusBadGateway,
	})
	failure := <-failures
	require.Equal(t, http.StatusBadGateway, failure.StatusCode)
	require.Equal(t, types.ErrorCode("upstream_task_failed"), failure.GetErrorCode())
	event := <-events
	assert.Equal(t, "relay", event.Component)
	assert.Equal(t, 12, event.ChannelId)
	assert.Equal(t, http.StatusBadGateway, event.StatusCode)
}
