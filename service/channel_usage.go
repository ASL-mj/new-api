package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const channelQuotaDisabledReason = "channel quota limit reached"

var recordChannelUsageSystemEvent = RecordSystemEvent

type ChannelUsageRecordParams struct {
	ChannelID      int
	SelectedKey    string
	KeyIndex       int
	HasKeyIdentity bool
	Quota          int
	TokenUsed      int64
	RequestCount   int64
	Now            time.Time
	ModelName      string
	Group          string
	RequestID      string
}

func RecordRelayChannelUsage(relayInfo *relaycommon.RelayInfo, quota int, tokenUsed int64, requestCount int64) error {
	if relayInfo == nil {
		return nil
	}

	params := ChannelUsageRecordParams{
		ChannelID:    relayInfo.ChannelId,
		Quota:        quota,
		TokenUsed:    tokenUsed,
		RequestCount: requestCount,
		ModelName:    relayInfo.OriginModelName,
		Group:        relayInfo.UsingGroup,
		RequestID:    relayInfo.RequestId,
	}
	if selectedKey, keyIndex, ok := relayInfo.GetChannelUsageIdentity(); ok {
		params.SelectedKey = selectedKey
		params.KeyIndex = keyIndex
		params.HasKeyIdentity = true
	}
	return RecordChannelUsage(params)
}

func RecordChannelUsage(params ChannelUsageRecordParams) error {
	if params.ChannelID <= 0 {
		return fmt.Errorf("invalid channel id: %d", params.ChannelID)
	}
	if params.Quota < 0 {
		params.Quota = 0
	}
	if params.TokenUsed < 0 {
		params.TokenUsed = 0
	}
	if params.RequestCount < 0 {
		params.RequestCount = 0
	}
	if params.Now.IsZero() {
		params.Now = time.Now()
	}
	if params.Quota == 0 && params.TokenUsed == 0 && params.RequestCount == 0 {
		return nil
	}
	if params.Quota > 0 && params.TokenUsed == 0 && params.RequestCount == 0 && !params.HasKeyIdentity && canBatchPureChannelUsage(params.ChannelID) {
		model.ApplyChannelUsedQuotaWithBatch(params.ChannelID, params.Quota)
		return nil
	}

	result, err := model.ApplyChannelUsageSettlement(model.ChannelUsageSettlementParams{
		ChannelID:      params.ChannelID,
		SelectedKey:    params.SelectedKey,
		KeyIndex:       params.KeyIndex,
		HasKeyIdentity: params.HasKeyIdentity && strings.TrimSpace(params.SelectedKey) != "",
		Quota:          params.Quota,
		TokenUsed:      params.TokenUsed,
		RequestCount:   params.RequestCount,
		Now:            params.Now,
	})
	if err != nil {
		return err
	}

	channelStatusPropagated := false
	channelAutoDisabledByKeyPropagation := false
	if result.Key != nil && result.Key.KeyJustExhausted {
		channelStatusPropagated = model.UpdateChannelStatus(
			params.ChannelID,
			params.SelectedKey,
			common.ChannelStatusAutoDisabled,
			model.ChannelKeyQuotaDisabledReason,
		)
		channelAutoDisabledByKeyPropagation = result.Key.ChannelJustExhausted
		recordKeyQuotaExhaustedEvent(params, *result.Key)
	}

	channelJustExhausted := result.Channel.ChannelJustExhausted || channelAutoDisabledByKeyPropagation
	if channelJustExhausted {
		if result.Channel.ChannelJustExhausted && !channelStatusPropagated {
			propagateChannelAutoDisabled(params.ChannelID)
		}
		recordChannelQuotaExhaustedEvent(params, result)
	}

	return nil
}

func canBatchPureChannelUsage(channelID int) bool {
	if !common.BatchUpdateEnabled || channelID <= 0 {
		return false
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil || channel == nil {
		return false
	}
	return !(channel.UsesChannelQuota() && channel.QuotaLimit > 0)
}

func propagateChannelAutoDisabled(channelID int) {
	if err := model.UpdateAbilityStatus(channelID, false); err != nil {
		common.SysLog(fmt.Sprintf("failed to disable abilities for exhausted channel: channel_id=%d, error=%v", channelID, err))
	}
	model.CacheUpdateChannelStatus(channelID, common.ChannelStatusAutoDisabled)
}

func recordKeyQuotaExhaustedEvent(params ChannelUsageRecordParams, result model.ChannelKeyUsageApplyResult) {
	recordChannelUsageEvent(
		"warn",
		"渠道 Key 已因额度耗尽自动禁用",
		params,
		map[string]interface{}{
			"event":            "key_quota_exhausted",
			"reason":           model.ChannelKeyQuotaDisabledReason,
			"key_index":        result.KeyIndex,
			"key_fingerprint":  result.KeyFingerprint,
			"quota_limit":      result.QuotaLimit,
			"quota_limit_used": result.QuotaLimitUsed,
		},
	)
}

func recordChannelQuotaExhaustedEvent(params ChannelUsageRecordParams, result model.ChannelUsageSettlementResult) {
	reason := channelQuotaDisabledReason
	extra := map[string]interface{}{
		"event":            "channel_quota_exhausted",
		"reason":           reason,
		"quota_limit":      result.Channel.QuotaLimit,
		"quota_limit_used": result.Channel.QuotaLimitUsed,
	}
	if result.Key != nil && result.Key.KeyJustExhausted {
		reason = model.ChannelKeyQuotaDisabledReason
		extra["reason"] = reason
		extra["key_index"] = result.Key.KeyIndex
		extra["key_fingerprint"] = result.Key.KeyFingerprint
	}

	recordChannelUsageEvent("warn", "渠道已因额度耗尽自动禁用", params, extra)
}

func recordChannelUsageEvent(level string, message string, params ChannelUsageRecordParams, extra map[string]interface{}) {
	event := model.SystemEventLog{
		CreatedAt: common.GetTimestamp(),
		Level:     level,
		Component: "channel_usage",
		Message:   message,
		RequestId: params.RequestID,
		ChannelId: params.ChannelID,
		ModelName: params.ModelName,
		Group:     params.Group,
	}
	if len(extra) > 0 {
		if payload, err := common.Marshal(extra); err == nil {
			event.Extra = string(payload)
		}
	}
	recordChannelUsageSystemEvent(event)
}
