package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestTaskPollingV2BackoffAndRetryAfter(t *testing.T) {
	config := taskPollingV2Config{MaxFailures: 20, BaseBackoffSeconds: 10, MaxBackoffSeconds: 300}
	for i, want := range []int64{10, 20, 40, 80, 160, 300, 300} {
		require.Equal(t, want, taskPollDelay(config, i+1, "", time.Unix(0, 0)))
	}
	require.EqualValues(t, 90, taskPollDelay(config, 1, "90", time.Unix(0, 0)))
	require.EqualValues(t, 10, taskPollDelay(config, 1, "301", time.Unix(0, 0)))
	require.EqualValues(t, 120, taskPollDelay(config, 1, time.Unix(120, 0).UTC().Format(http.TimeFormat), time.Unix(0, 0)))
	for code, class := range map[int]string{200: "", 302: "malformed_response", 408: "transport", 429: "rate_limited", 500: "server_transient", 403: "provider_terminal"} {
		require.Equal(t, class, taskPollHTTPClass(code))
	}
}

func TestTaskPollingV2RetriesThenRefundsOnce(t *testing.T) {
	task := settlementFixture(t, false)
	config := taskPollingV2Config{MaxFailures: 20, BaseBackoffSeconds: 10, MaxBackoffSeconds: 300}
	for i := 1; i <= 20; i++ {
		now := time.Unix(int64(i)*1000, 0)
		reloaded, err := model.GetTaskByID(task.ID)
		require.NoError(t, err)
		won, err := model.ClaimTaskPoll(reloaded, now.Unix(), now.Unix()+300)
		require.NoError(t, err)
		require.True(t, won)
		require.NoError(t, recordTaskPollFailure(context.Background(), reloaded, config, "transport", "", now))
		stored, err := model.GetTaskByID(task.ID)
		require.NoError(t, err)
		require.Equal(t, i, stored.PollFailures)
		if i < 20 {
			require.EqualValues(t, model.TaskStatusInProgress, stored.Status)
			require.Equal(t, 1000, getUserQuota(t, 201))
		} else {
			require.EqualValues(t, model.TaskStatusFailure, stored.Status)
			require.Equal(t, "poll_exhausted", stored.LastPollErrorClass)
			require.Equal(t, 1300, getUserQuota(t, 201))
		}
	}
}

func TestTaskPollingV2InternalFailureRetainsFunds(t *testing.T) {
	task := settlementFixture(t, false)
	task.PollFailures = 50
	require.NoError(t, recordTaskPollFailure(context.Background(), task, taskPollingV2Config{MaxFailures: 20, BaseBackoffSeconds: 10, MaxBackoffSeconds: 300}, "internal", "", time.Unix(100, 0)))
	stored, err := model.GetTaskByID(task.ID)
	require.NoError(t, err)
	require.EqualValues(t, model.TaskStatusInProgress, stored.Status)
	require.Equal(t, 1000, getUserQuota(t, 201))
}

type v2PollingFixture struct {
	result *relaycommon.TaskInfo
	status int
	body   string
}

func (a *v2PollingFixture) Init(*relaycommon.RelayInfo) {}
func (a *v2PollingFixture) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{StatusCode: a.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(a.body))}, nil
}
func (a *v2PollingFixture) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return a.result, nil
}
func (a *v2PollingFixture) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int { return 0 }

func TestTaskPollingV2SuccessfulPollResetsFailures(t *testing.T) {
	task := settlementFixture(t, false)
	task.PollFailures = 8
	require.NoError(t, model.DB.Model(task).Update("poll_failures", 8).Error)
	result := &relaycommon.TaskInfo{Status: model.TaskStatusInProgress, Progress: "50%"}
	require.NoError(t, applyTaskPollResult(context.Background(), &v2PollingFixture{}, task, result, []byte(`{}`), loadTaskPollingV2Config(), time.Unix(100, 0)))
	stored, err := model.GetTaskByID(task.ID)
	require.NoError(t, err)
	require.Zero(t, stored.PollFailures)
	require.EqualValues(t, 115, stored.NextPollAt)
	require.Equal(t, "50%", stored.Progress)
}

func TestTaskPollingV2HTTPAndMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		result *relaycommon.TaskInfo
		class  string
	}{
		{"rate limit", 429, nil, "rate_limited"},
		{"server", 503, nil, "server_transient"},
		{"empty result", 200, nil, "malformed_response"},
		{"unknown status", 200, &relaycommon.TaskInfo{Status: "mystery"}, "malformed_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := settlementFixture(t, false)
			original := GetTaskAdaptorFunc
			GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor {
				return &v2PollingFixture{status: tc.status, result: tc.result}
			}
			t.Cleanup(func() { GetTaskAdaptorFunc = original })
			before := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = false
			t.Cleanup(func() { common.MemoryCacheEnabled = before })
			require.NoError(t, pollTaskV2(context.Background(), task, loadTaskPollingV2Config(), time.Unix(100, 0)))
			stored, err := model.GetTaskByID(task.ID)
			require.NoError(t, err)
			require.Equal(t, tc.class, stored.LastPollErrorClass)
			require.Equal(t, 1000, getUserQuota(t, 201))
		})
	}
}
