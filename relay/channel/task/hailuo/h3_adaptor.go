package hailuo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/task_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func (a *TaskAdaptor) PrepareTaskBilling(c *gin.Context, info *relaycommon.RelayInfo) (bool, error) {
	alreadyFrozen := info.TaskRelayInfo != nil && info.TaskPricingSnapshot != nil && info.Billing != nil
	frozenGroup := info.PriceData.GroupRatioInfo
	if err := a.PrepareTaskPricing(c, info); err != nil {
		return IsH3ModelName(info.UpstreamModelName), err
	}
	if !IsH3ModelName(info.UpstreamModelName) {
		return false, nil
	}
	group := helper.HandleGroupRatio(c, info)
	if alreadyFrozen {
		group = frozenGroup
	}
	snapshot := info.TaskPricingSnapshot
	quota, err := snapshot.Quota(snapshot.Submitted, group.GroupRatio)
	if err != nil {
		return true, err
	}
	ratios, err := snapshot.OtherRatios()
	if err != nil {
		return true, err
	}
	info.PriceData = types.PriceData{Quota: quota, UsePrice: true, FreeModel: group.GroupRatio == 0,
		ModelPrice: float64(snapshot.BaseQuota) / common.QuotaPerUnit, OtherRatios: ratios, GroupRatioInfo: group}
	return true, nil
}

// PrepareTaskPricing is called AFTER channel model mapping and BEFORE precharge.
// Explicit channel mappings may target MiniMax-H3; aliases alone never select v2.
func (a *TaskAdaptor) PrepareTaskPricing(c *gin.Context, info *relaycommon.RelayInfo) error {
	a.upstreamModel = info.UpstreamModelName
	if !IsH3ModelName(info.UpstreamModelName) {
		a.h3Request = nil
		if info.TaskRelayInfo != nil {
			info.TaskPricingSnapshot = nil
		}
		if c.GetBool("h3_deferred_validation") {
			return fmt.Errorf("H3 request must map to MiniMax-H3")
		}
		return nil
	}
	if !task_setting.H3Enabled() || !service.TaskPollingV2EnabledForChannel(info.ChannelId) {
		return fmt.Errorf("H3 new submissions are disabled for this channel")
	}
	if info.TaskRelayInfo == nil {
		return fmt.Errorf("H3 task relay context is missing")
	}
	var raw json.RawMessage
	if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
		return fmt.Errorf("invalid H3 request")
	}
	request, facts, err := decodeH3Request(raw)
	if err != nil {
		return err
	}
	if info.TaskPricingSnapshot != nil {
		if err := info.TaskPricingSnapshot.Validate(); err != nil {
			return err
		}
		if facts != info.TaskPricingSnapshot.Submitted {
			return fmt.Errorf("H3 retry changed frozen usage facts")
		}
	} else {
		cfg, err := task_setting.GetH3PricingRules()
		if err != nil {
			return err
		}
		snapshot, err := types.NewH3PricingSnapshot(cfg.Version, cfg.BaseQuota, cfg.H3PricingRule, facts)
		if err != nil {
			return err
		}
		info.TaskPricingSnapshot = snapshot
	}
	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	if err != nil {
		return fmt.Errorf("H3 media client unavailable")
	}
	if err := validateH3Media(c.Request.Context(), request, client); err != nil {
		return err
	}
	a.h3Request = request
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if info.TaskRelayInfo != nil && info.TaskPricingSnapshot != nil {
		ratios, err := info.TaskPricingSnapshot.OtherRatios()
		if err == nil {
			return ratios
		}
	}
	return nil
}

func (a *TaskAdaptor) h3Body(info *relaycommon.RelayInfo) (io.Reader, error) {
	if !task_setting.H3Enabled() || !service.TaskPollingV2EnabledForChannel(info.ChannelId) {
		return nil, fmt.Errorf("H3 new submissions are disabled for this channel")
	}
	if a.h3Request == nil || info.TaskRelayInfo == nil || info.TaskPricingSnapshot == nil {
		return nil, fmt.Errorf("H3 pricing and media must be prepared before submission")
	}
	if err := info.TaskPricingSnapshot.Validate(); err != nil {
		return nil, err
	}
	data, err := common.Marshal(a.h3Request)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) h3DoRequest(c *gin.Context, info *relaycommon.RelayInfo) (*http.Response, error) {
	body, err := a.h3Body(info)
	if err != nil {
		return nil, err
	}
	client, err := h3ProviderClient(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}
	uri := strings.TrimRight(a.baseURL, "/") + H3SubmitEndpoint
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, uri, body)
	if err != nil {
		return nil, fmt.Errorf("invalid H3 upstream URL")
	}
	if err := a.BuildRequestHeader(c, req, info); err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return resp, &H3SubmissionOutcomeUnknownError{}
	}
	return resp, nil
}

func (a *TaskAdaptor) h3DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	if resp == nil || resp.Body == nil {
		return "", nil, h3UnknownSubmission()
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return "", nil, h3UnknownSubmission()
	}
	if resp.StatusCode == 408 || resp.StatusCode >= 500 {
		return "", nil, h3UnknownSubmission()
	}
	var response h3CreateResponse
	if err := common.Unmarshal(data, &response); err != nil {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("%s", h3ErrorMessage(resp.StatusCode)), "h3_upstream_error", resp.StatusCode)
		}
		return "", nil, h3UnknownSubmission()
	}
	if response.Error != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := resp.StatusCode
		if status := response.Error.statusCode(); status != 0 {
			code = status
		}
		if code < 400 || code > 599 {
			return "", nil, h3UnknownSubmission()
		}
		if code == 408 || code >= 500 {
			return "", nil, h3UnknownSubmission()
		}
		return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("%s", h3ErrorMessage(code)), "h3_upstream_error", code)
	}
	if !validH3TaskID(response.TaskID) {
		return "", nil, h3UnknownSubmission()
	}
	// Controller owns the public response, after upstream binding and durable
	// billing bookkeeping succeed. Never acknowledge the task here.
	stored, _ := common.Marshal(map[string]string{"task_id": response.TaskID})
	return response.TaskID, stored, nil
}

const H3SubmitOutcomeUnknownCode = "h3_submit_outcome_unknown"

// H3SubmissionOutcomeUnknownError marks a request that may have been accepted.
// It deliberately excludes transport error strings containing keys or URLs.
type H3SubmissionOutcomeUnknownError struct{}

func (*H3SubmissionOutcomeUnknownError) Error() string {
	return "H3 submission outcome is unknown; reconciliation is required"
}
func (*H3SubmissionOutcomeUnknownError) SubmissionOutcomeUnknown() bool { return true }

func h3UnknownSubmission() *dto.TaskError {
	return service.TaskErrorWrapperLocal(&H3SubmissionOutcomeUnknownError{}, H3SubmitOutcomeUnknownCode, http.StatusBadGateway)
}

func (a *TaskAdaptor) h3FetchTask(baseURL, key, taskID, proxy string) (*http.Response, error) {
	if !validH3TaskID(taskID) {
		return nil, fmt.Errorf("invalid H3 task ID")
	}
	a.expectedTaskID = taskID
	client, err := h3ProviderClient(proxy)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+H3QueryEndpoint+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid H3 query URL")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	return client.Do(req)
}

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, result *relaycommon.TaskInfo) int {
	quota, err := a.H3CompletionQuota(task, result)
	if err != nil {
		return 0
	}
	return quota
}

// H3CompletionQuota exposes errors to settlement owners. Do not silently fall
// through to token settlement on a corrupt/missing H3 snapshot.
func (a *TaskAdaptor) H3CompletionQuota(task *model.Task, result *relaycommon.TaskInfo) (int, error) {
	if task == nil || result == nil || result.Status != model.TaskStatusSuccess {
		return 0, fmt.Errorf("H3 completion is not successful")
	}
	bc := task.PrivateData.BillingContext
	if bc == nil || bc.TaskPricingSnapshot == nil {
		return 0, fmt.Errorf("H3 pricing snapshot is missing")
	}
	var usage types.H3UsageOverrides
	if result.TaskUsage != nil {
		usage = *result.TaskUsage
	}
	facts := usage.Merge(bc.TaskPricingSnapshot.Submitted)
	return bc.TaskPricingSnapshot.Quota(facts, bc.GroupRatio)
}

func (a *TaskAdaptor) validateH3Deferred(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("invalid task request"), "invalid_request", http.StatusBadRequest)
	}
	info.Action = constant.TaskActionGenerate
	c.Set("task_request", req)
	c.Set("h3_deferred_validation", true)
	return nil
}
