package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const taskPollLeaseSeconds int64 = 300
const taskPollBodyLimit int64 = 4 << 20

func taskPollDelay(config taskPollingV2Config, failures int, retryAfter string, now time.Time) int64 {
	shift := failures - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 5 {
		shift = 5
	}
	delay := math.Min(float64(config.MaxBackoffSeconds), float64(config.BaseBackoffSeconds)*math.Pow(2, float64(shift)))
	retry, err := strconv.ParseInt(strings.TrimSpace(retryAfter), 10, 64)
	if err != nil {
		if until, parseErr := http.ParseTime(retryAfter); parseErr == nil {
			retry = int64(until.Sub(now).Seconds())
		}
	}
	if retry > 0 && retry <= 300 && float64(retry) > delay {
		delay = float64(retry)
	}
	return int64(delay)
}

func taskPollHTTPClass(status int) string {
	switch {
	case status == 408:
		return "transport"
	case status == 429:
		return "rate_limited"
	case status >= 500:
		return "server_transient"
	case status >= 400:
		return "provider_terminal"
	case status < 200 || status >= 300:
		return "malformed_response"
	default:
		return ""
	}
}

// Diagnostic messages are fixed vocabulary: upstream bodies/URLs may contain secrets.
func taskPollMessage(class string) string {
	switch class {
	case "transport":
		return "Upstream connection interrupted"
	case "rate_limited":
		return "Upstream rate limit reached"
	case "server_transient":
		return "Upstream temporarily unavailable"
	case "malformed_response":
		return "Upstream task response is invalid"
	case "provider_terminal":
		return "Upstream task failed or was cancelled"
	case "poll_exhausted":
		return "Task polling retry limit reached"
	default:
		return "Task requires internal configuration or storage repair"
	}
}

func recordTaskPollFailure(ctx context.Context, task *model.Task, config taskPollingV2Config, class, retryAfter string, now time.Time) error {
	countTaskPollFailure(class)
	from, leaseUntil := task.Status, task.NextPollAt
	task.PollFailures++
	task.LastPollErrorClass = class
	task.LastPollErrorMessage = taskPollMessage(class)
	if class == "provider_terminal" || (class != "internal" && task.PollFailures >= config.MaxFailures) {
		if class != "provider_terminal" {
			class = "poll_exhausted"
		}
		task.LastPollErrorClass = class
		task.FailReason = taskPollMessage(class)
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now.Unix()
		return SettleTaskTerminal(ctx, task, from, 0, 0, 0, class)
	}
	task.NextPollAt = now.Unix() + taskPollDelay(config, task.PollFailures, retryAfter, now)
	_, err := model.SaveTaskPoll(task, from, leaseUntil)
	return err
}

func taskPollKey(task *model.Task, ch *model.Channel) (string, error) {
	if fingerprint := task.PrivateData.ChannelKeyFingerprint; fingerprint != "" {
		for _, key := range ch.GetKeys() {
			candidate, err := model.FingerprintChannelKey(key)
			if err != nil {
				return "", err
			}
			if candidate == fingerprint {
				return key, nil
			}
		}
		return "", fmt.Errorf("original task credential no longer configured")
	}
	if task.PrivateData.Key != "" {
		return task.PrivateData.Key, nil
	}
	keys := ch.GetKeys()
	if len(keys) == 1 {
		return keys[0], nil
	}
	return "", fmt.Errorf("task credential identity missing")
}

func runTaskPollingV2(ctx context.Context) error {
	config := loadTaskPollingV2Config()
	if err := RetryTaskSubmissionAccounting(ctx, 100); err != nil {
		logger.LogError(ctx, "task submission accounting recovery failed")
	}
	if err := RetryTaskSettlementEffects(ctx, 100); err != nil {
		logger.LogError(ctx, "task settlement effects retry failed")
	}
	tasks, err := model.GetReadyTaskPolls(time.Now().Unix(), constant.TaskQueryLimit)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		now := time.Now()
		won, claimErr := model.ClaimTaskPoll(task, now.Unix(), now.Unix()+taskPollLeaseSeconds)
		if claimErr != nil {
			return claimErr
		}
		if !won {
			continue
		}
		if err := pollTaskV2(ctx, task, config, now); err != nil {
			logger.LogError(ctx, fmt.Sprintf("task %s poll persistence failed", task.TaskID))
		}
	}
	return nil
}

func pollTaskV2(ctx context.Context, task *model.Task, config taskPollingV2Config, now time.Time) error {
	ch, err := model.CacheGetChannel(task.ChannelId)
	if err != nil || GetTaskAdaptorFunc == nil {
		return recordTaskPollFailure(ctx, task, config, "internal", "", now)
	}
	adaptor := GetTaskAdaptorFunc(task.Platform)
	if adaptor == nil {
		return recordTaskPollFailure(ctx, task, config, "internal", "", now)
	}
	key, err := taskPollKey(task, ch)
	if err != nil {
		return recordTaskPollFailure(ctx, task, config, "internal", "", now)
	}
	modelName := task.Properties.UpstreamModelName
	if modelName == "" {
		modelName = task.Properties.OriginModelName
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: task.Properties.OriginModelName,
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: ch.Type, ChannelBaseUrl: ch.GetBaseURL(), ApiKey: key, UpstreamModelName: modelName},
	}
	adaptor.Init(info)
	body := map[string]any{"task_id": task.GetUpstreamTaskID(), "action": task.Action, "model": modelName}
	if task.Platform == constant.TaskPlatformSuno {
		body["ids"] = []string{task.GetUpstreamTaskID()}
	}
	resp, err := adaptor.FetchTask(ch.GetBaseURL(), key, body, ch.GetSetting().Proxy)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return recordTaskPollFailure(ctx, task, config, "transport", "", now)
	}
	if resp == nil || resp.Body == nil {
		return recordTaskPollFailure(ctx, task, config, "malformed_response", "", now)
	}
	defer resp.Body.Close()
	if class := taskPollHTTPClass(resp.StatusCode); class != "" {
		return recordTaskPollFailure(ctx, task, config, class, resp.Header.Get("Retry-After"), now)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, taskPollBodyLimit+1))
	if err != nil {
		return recordTaskPollFailure(ctx, task, config, "transport", "", now)
	}
	if int64(len(data)) > taskPollBodyLimit {
		return recordTaskPollFailure(ctx, task, config, "malformed_response", "", now)
	}
	result, err := parseTaskPollResult(adaptor, task, data)
	if err != nil || result == nil {
		return recordTaskPollFailure(ctx, task, config, "malformed_response", "", now)
	}
	if class := taskPollHTTPClass(result.Code); result.Code >= 400 && class != "" {
		return recordTaskPollFailure(ctx, task, config, class, resp.Header.Get("Retry-After"), now)
	}
	return applyTaskPollResult(ctx, adaptor, task, result, data, config, now)
}

func parseTaskPollResult(adaptor TaskPollingAdaptor, task *model.Task, data []byte) (*relaycommon.TaskInfo, error) {
	if task.Platform == constant.TaskPlatformSuno {
		var response dto.TaskResponse[[]dto.SunoDataResponse]
		if err := common.Unmarshal(data, &response); err != nil {
			return nil, err
		}
		if !response.IsSuccess() {
			return nil, fmt.Errorf("invalid suno response")
		}
		for _, item := range response.Data {
			if item.TaskID == task.GetUpstreamTaskID() {
				return &relaycommon.TaskInfo{Status: item.Status, Reason: item.FailReason}, nil
			}
		}
		return nil, fmt.Errorf("task absent from response")
	}
	var envelope dto.TaskResponse[model.Task]
	if err := common.Unmarshal(data, &envelope); err == nil && envelope.IsSuccess() {
		t := envelope.Data
		return &relaycommon.TaskInfo{Status: string(t.Status), Url: t.GetResultURL(), Progress: t.Progress, Reason: t.FailReason}, nil
	}
	return adaptor.ParseTaskResult(data)
}

func applyTaskPollResult(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, result *relaycommon.TaskInfo, data []byte, config taskPollingV2Config, now time.Time) error {
	from, leaseUntil := task.Status, task.NextPollAt
	switch result.Status {
	case model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusInProgress, model.TaskStatusSuccess, model.TaskStatusFailure:
	default:
		return recordTaskPollFailure(ctx, task, config, "malformed_response", "", now)
	}
	task.Status = model.TaskStatus(result.Status)
	task.PollFailures = 0
	task.LastPollErrorClass, task.LastPollErrorMessage = "", ""
	task.Data = redactVideoResponseBody(data)
	task.Progress = result.Progress
	if task.Progress == "" {
		task.Progress = "30%"
	}
	if task.Status == model.TaskStatusInProgress && task.StartTime == 0 {
		task.StartTime = now.Unix()
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		task.NextPollAt = now.Unix() + 15
		_, err := model.SaveTaskPoll(task, from, leaseUntil)
		return err
	}
	task.Progress, task.FinishTime = "100%", now.Unix()
	if task.Status == model.TaskStatusFailure {
		task.FailReason = taskPollMessage("provider_terminal")
		return SettleTaskTerminal(ctx, task, from, 0, 0, 0, "provider_terminal")
	}
	task.PrivateData.ResultURL = result.Url
	actual, standard := task.Quota, task.PrivateData.ChannelStandardQuota
	bc := task.PrivateData.BillingContext
	if bc != nil && bc.TaskPricingSnapshot != nil {
		facts := bc.TaskPricingSnapshot.Submitted
		if result.TaskUsage != nil {
			facts = result.TaskUsage.Merge(facts)
		}
		var err error
		standard, err = bc.TaskPricingSnapshot.StandardQuota(facts)
		if err == nil {
			actual, err = bc.TaskPricingSnapshot.Quota(facts, bc.GroupRatio)
		}
		if err != nil {
			task.Status = from
			return recordTaskPollFailure(ctx, task, config, "internal", "", now)
		}
	} else if bc != nil && !bc.PerCallBilling {
		if value := adaptor.AdjustBillingOnComplete(task, result); value > 0 {
			actual = value
			if bc.GroupRatio > 0 {
				standard = int(math.Round(float64(actual) / bc.GroupRatio))
			}
		} else if result.TotalTokens > 0 {
			multiplier := 1.0
			for _, ratio := range bc.OtherRatios {
				multiplier *= ratio
			}
			standard = int(float64(result.TotalTokens) * bc.ModelRatio * multiplier)
			actual = int(float64(standard) * bc.GroupRatio)
		}
	}
	return SettleTaskTerminal(ctx, task, from, actual, standard, int64(result.TotalTokens), "provider_terminal")
}
