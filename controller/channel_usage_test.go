package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performChannelUsageRequest(t *testing.T, method, target, body string, params map[string]string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	return performChannelUsageRequestWithLanguage(t, method, target, body, params, "zh-CN", handler)
}

func performChannelUsageRequestWithLanguage(t *testing.T, method, target, body string, params map[string]string, language string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("Accept-Language", language)
	for key, value := range params {
		ctx.AddParam(key, value)
	}
	handler(ctx)
	return recorder
}

func TestChannelUsageAPIBatchStatsAndValidation(t *testing.T) {
	db := setupChannelQuotaGuardControllerTestDB(t)
	channel := &model.Channel{Id: 501, Name: "usage-batch", Key: "sk-secret", Balance: 12.5, QuotaLimit: 900, QuotaLimitUsed: 300}
	require.NoError(t, db.Create(channel).Error)
	today, _, err := model.GetChannelUsageDateRange(time.Now(), 30)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ChannelUsageDaily{ChannelId: channel.Id, UsageDate: today, Quota: 80, TokenUsed: 20, RequestCount: 3}).Error)

	recorder := performChannelUsageRequest(t, http.MethodGet, "/api/channel/usage/batch?ids=501,999", "", nil, GetChannelUsageBatch)
	response := decodeChannelQuotaGuardResponse(t, recorder.Body.String())
	require.Equal(t, true, response["success"])
	assert.Contains(t, recorder.Body.String(), `"501":{"channel_id":501,"today_quota":80`)
	assert.Contains(t, recorder.Body.String(), `"999":{"channel_id":999,"today_quota":0`)
	assert.NotContains(t, recorder.Body.String(), "sk-secret")

	tooMany := make([]string, 201)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("%d", index+1)
	}
	invalid := performChannelUsageRequest(t, http.MethodGet, "/api/channel/usage/batch?ids="+strings.Join(tooMany, ","), "", nil, GetChannelUsageBatch)
	invalidResponse := decodeChannelQuotaGuardResponse(t, invalid.Body.String())
	assert.Equal(t, false, invalidResponse["success"])
	assert.Contains(t, invalidResponse["message"], "200")
}

func TestChannelQuotaResetAPIClearsUsageWithoutEnabling(t *testing.T) {
	db := setupChannelQuotaGuardControllerTestDB(t)
	channel := &model.Channel{
		Id: 502, Name: "quota-reset", Key: "sk-reset", Status: common.ChannelStatusAutoDisabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeChannel, QuotaLimit: 100, QuotaLimitUsed: 100,
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := performChannelUsageRequest(t, http.MethodPost, "/api/channel/502/quota/reset", "", map[string]string{"id": "502"}, ResetChannelQuotaUsage)
	response := decodeChannelQuotaGuardResponse(t, recorder.Body.String())
	require.Equal(t, true, response["success"])

	var reloaded model.Channel
	require.NoError(t, db.First(&reloaded, channel.Id).Error)
	assert.Zero(t, reloaded.QuotaLimitUsed)
	assert.NotZero(t, reloaded.QuotaLimitResetAt)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
}

func TestChannelKeyUsageAPIsDoNotExposeKeysAndKeepStatusOnResetOrLimitUpdate(t *testing.T) {
	db := setupChannelQuotaGuardControllerTestDB(t)
	channel := &model.Channel{
		Id: 503, Name: "key-usage", Key: "sk-alpha-secret\nsk-beta-secret", Status: common.ChannelStatusAutoDisabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeKey,
		ChannelInfo:    model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2},
	}
	require.NoError(t, db.Create(channel).Error)
	current, err := model.EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	usage := current[0]
	require.NoError(t, db.Model(&model.ChannelKeyUsage{}).Where("id = ?", usage.Id).Updates(map[string]interface{}{
		"quota_limit": 50, "quota_limit_used": 50, "status": common.ChannelStatusAutoDisabled,
		"disabled_reason": model.ChannelKeyQuotaDisabledReason,
	}).Error)

	list := performChannelUsageRequest(t, http.MethodGet, "/api/channel/503/key-usages", "", map[string]string{"id": "503"}, GetChannelKeyUsageList)
	listResponse := decodeChannelQuotaGuardResponse(t, list.Body.String())
	require.Equal(t, true, listResponse["success"])
	assert.Contains(t, list.Body.String(), usage.KeyFingerprint)
	assert.Contains(t, list.Body.String(), usage.KeyMask)
	assert.NotContains(t, list.Body.String(), "sk-alpha-secret")
	assert.NotContains(t, list.Body.String(), "sk-beta-secret")

	params := map[string]string{"id": "503", "fingerprint": usage.KeyFingerprint}
	reset := performChannelUsageRequest(t, http.MethodPost, "/api/channel/503/key-usages/key/reset", "", params, ResetChannelKeyQuotaUsage)
	resetResponse := decodeChannelQuotaGuardResponse(t, reset.Body.String())
	require.Equal(t, true, resetResponse["success"])

	limit := performChannelUsageRequest(t, http.MethodPut, "/api/channel/503/key-usages/key/limit", `{"quota_limit":75}`, params, UpdateChannelKeyQuotaLimit)
	limitResponse := decodeChannelQuotaGuardResponse(t, limit.Body.String())
	require.Equal(t, true, limitResponse["success"])

	var reloaded model.ChannelKeyUsage
	require.NoError(t, db.First(&reloaded, usage.Id).Error)
	assert.Zero(t, reloaded.QuotaLimitUsed)
	assert.EqualValues(t, 75, reloaded.QuotaLimit)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	assert.Equal(t, model.ChannelKeyQuotaDisabledReason, reloaded.DisabledReason)

	negative := performChannelUsageRequest(t, http.MethodPut, "/api/channel/503/key-usages/key/limit", `{"quota_limit":-1}`, params, UpdateChannelKeyQuotaLimit)
	negativeResponse := decodeChannelQuotaGuardResponse(t, negative.Body.String())
	assert.Equal(t, false, negativeResponse["success"])
	assert.Contains(t, negativeResponse["message"], "不能小于")

	unknownParams := map[string]string{"id": "503", "fingerprint": strings.Repeat("a", 64)}
	unknown := performChannelUsageRequest(t, http.MethodPost, "/api/channel/503/key-usages/unknown/reset", "", unknownParams, ResetChannelKeyQuotaUsage)
	unknownResponse := decodeChannelQuotaGuardResponse(t, unknown.Body.String())
	assert.Equal(t, false, unknownResponse["success"])
	assert.Contains(t, unknownResponse["message"], "不存在")
}

func TestChannelQuotaConfigurationValidation(t *testing.T) {
	assert.Error(t, validateChannel(&model.Channel{QuotaLimitMode: "invalid"}, true))
	assert.Error(t, validateChannel(&model.Channel{QuotaLimitMode: model.ChannelQuotaLimitModeChannel, QuotaLimit: -1}, true))
	assert.Error(t, validateChannel(&model.Channel{QuotaLimitMode: model.ChannelQuotaLimitModeKey, Key: "sk-single"}, true))
	assert.NoError(t, validateChannel(&model.Channel{
		Key: "sk-a\nsk-b", QuotaLimitMode: model.ChannelQuotaLimitModeBoth,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2},
	}, true))
}

func TestChannelUsageAPIMessagesAreLocalized(t *testing.T) {
	db := setupChannelQuotaGuardControllerTestDB(t)
	channel := &model.Channel{
		Id: 504, Name: "localized-quota", Key: "sk-localized",
		QuotaLimitMode: model.ChannelQuotaLimitModeChannel, QuotaLimit: 100, QuotaLimitUsed: 50,
	}
	require.NoError(t, db.Create(channel).Error)

	reset := performChannelUsageRequestWithLanguage(
		t, http.MethodPost, "/api/channel/504/quota/reset", "",
		map[string]string{"id": "504"}, "en", ResetChannelQuotaUsage,
	)
	resetResponse := decodeChannelQuotaGuardResponse(t, reset.Body.String())
	assert.Equal(t, "Channel quota usage has been reset", resetResponse["message"])

	english := performChannelUsageRequestWithLanguage(
		t, http.MethodGet, "/api/channel/usage/batch?ids=", "", nil, "en", GetChannelUsageBatch,
	)
	assert.Contains(t, english.Body.String(), "Channel IDs cannot be empty")

	traditional := performChannelUsageRequestWithLanguage(
		t, http.MethodGet, "/api/channel/usage/batch?ids=", "", nil, "zh-TW", GetChannelUsageBatch,
	)
	assert.Contains(t, traditional.Body.String(), "管道 ID 不能為空")
}
