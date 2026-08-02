package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	backendI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	if err := backendI18n.Init(); err != nil {
		panic(err)
	}
}

func performChannelI18nRequest(t *testing.T, method, target, language string, params map[string]string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	ctx.Request.Header.Set("Accept-Language", language)
	for key, value := range params {
		ctx.AddParam(key, value)
	}
	handler(ctx)
	return recorder
}

func decodeChannelI18nResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	response := make(map[string]any)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestValidateChannelCustomMessagesAreLocalized(t *testing.T) {
	testCases := []struct {
		name     string
		channel  *model.Channel
		expected map[string]string
	}{
		{
			name:    "channel is required",
			channel: nil,
			expected: map[string]string{
				"zh-CN": "无效的参数",
				"zh-TW": "無效的參數",
				"en":    "Invalid parameters",
			},
		},
		{
			name:    "quota limit negative",
			channel: &model.Channel{QuotaLimit: -1},
			expected: map[string]string{
				"zh-CN": "渠道限额不能小于 0",
				"zh-TW": "管道限額不能小於 0",
				"en":    "Channel quota cannot be less than 0",
			},
		},
		{
			name:    "quota mode invalid",
			channel: &model.Channel{QuotaLimitMode: "invalid"},
			expected: map[string]string{
				"zh-CN": "无效的渠道限额模式",
				"zh-TW": "無效的管道限額模式",
				"en":    "Invalid channel quota mode",
			},
		},
		{
			name: "single key quota unsupported",
			channel: &model.Channel{
				Key:            "sk-single",
				QuotaLimitMode: model.ChannelQuotaLimitModeKey,
			},
			expected: map[string]string{
				"zh-CN": "单密钥渠道不支持密钥限额模式",
				"zh-TW": "單密鑰管道不支援密鑰限額模式",
				"en":    "Single-key channels do not support per-key quota mode",
			},
		},
		{
			name: "codex key must be json",
			channel: &model.Channel{
				Type: constant.ChannelTypeCodex,
				Key:  "plain-text",
			},
			expected: map[string]string{
				"zh-CN": "Codex 渠道密钥必须是合法的 JSON 对象",
				"zh-TW": "Codex 管道密鑰必須是合法的 JSON 物件",
				"en":    "Codex channel key must be a valid JSON object",
			},
		},
		{
			name: "codex key access token required",
			channel: &model.Channel{
				Type: constant.ChannelTypeCodex,
				Key:  `{"account_id":"acct_123","refresh_token":"rt_123"}`,
			},
			expected: map[string]string{
				"zh-CN": "Codex 渠道密钥 JSON 必须包含 access_token",
				"zh-TW": "Codex 管道密鑰 JSON 必須包含 access_token",
				"en":    "Codex channel key JSON must include access_token",
			},
		},
		{
			name: "codex key account id required",
			channel: &model.Channel{
				Type: constant.ChannelTypeCodex,
				Key:  `{"access_token":"at_123","refresh_token":"rt_123"}`,
			},
			expected: map[string]string{
				"zh-CN": "Codex 渠道密钥 JSON 必须包含 account_id",
				"zh-TW": "Codex 管道密鑰 JSON 必須包含 account_id",
				"en":    "Codex channel key JSON must include account_id",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for language, expected := range testCase.expected {
				t.Run(language, func(t *testing.T) {
					err := validateChannel(testCase.channel, true)
					require.Error(t, err)

					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", nil)
					ctx.Request.Header.Set("Accept-Language", language)

					common.ApiError(ctx, err)

					response := decodeChannelI18nResponse(t, recorder)
					assert.Equal(t, expected, response["message"])
				})
			}
		})
	}
}

func TestRefreshCodexChannelCredentialMessagesAreLocalized(t *testing.T) {
	originalRefresh := refreshCodexChannelCredential
	t.Cleanup(func() {
		refreshCodexChannelCredential = originalRefresh
	})

	t.Run("invalid channel id", func(t *testing.T) {
		expected := map[string]string{
			"zh-CN": "渠道ID格式错误",
			"zh-TW": "管道ID格式錯誤",
			"en":    "Channel ID format error",
		}
		for language, message := range expected {
			recorder := performChannelI18nRequest(
				t, http.MethodPost, "/api/channel/invalid/codex/refresh", language,
				map[string]string{"id": "invalid"}, RefreshCodexChannelCredential,
			)
			response := decodeChannelI18nResponse(t, recorder)
			assert.Equal(t, message, response["message"])
		}
	})

	t.Run("refresh failed", func(t *testing.T) {
		refreshCodexChannelCredential = func(ctx context.Context, channelID int, opts service.CodexCredentialRefreshOptions) (*service.CodexOAuthKey, *model.Channel, error) {
			return nil, nil, errors.New("boom")
		}

		expected := map[string]string{
			"zh-CN": "刷新 Codex 渠道凭证失败，请稍后重试",
			"zh-TW": "刷新 Codex 管道憑證失敗，請稍後重試",
			"en":    "Failed to refresh the Codex channel credential. Please try again later",
		}
		for language, message := range expected {
			recorder := performChannelI18nRequest(
				t, http.MethodPost, "/api/channel/7/codex/refresh", language,
				map[string]string{"id": "7"}, RefreshCodexChannelCredential,
			)
			response := decodeChannelI18nResponse(t, recorder)
			assert.Equal(t, message, response["message"])
		}
	})

	t.Run("refresh success", func(t *testing.T) {
		refreshCodexChannelCredential = func(ctx context.Context, channelID int, opts service.CodexCredentialRefreshOptions) (*service.CodexOAuthKey, *model.Channel, error) {
			return &service.CodexOAuthKey{
					AccessToken: "at_123",
					AccountID:   "acct_123",
					Email:       "codex@example.com",
					LastRefresh: "2026-08-02T08:00:00Z",
					Expired:     "2026-08-03T08:00:00Z",
				}, &model.Channel{
					Id:   channelID,
					Type: constant.ChannelTypeCodex,
					Name: "Codex Channel",
				}, nil
		}

		expected := map[string]string{
			"zh-CN": "Codex 渠道凭证已刷新",
			"zh-TW": "Codex 管道憑證已刷新",
			"en":    "Codex channel credential refreshed",
		}
		for language, message := range expected {
			recorder := performChannelI18nRequest(
				t, http.MethodPost, "/api/channel/7/codex/refresh", language,
				map[string]string{"id": "7"}, RefreshCodexChannelCredential,
			)
			response := decodeChannelI18nResponse(t, recorder)
			assert.Equal(t, true, response["success"])
			assert.Equal(t, message, response["message"])
			assert.Contains(t, recorder.Body.String(), `"account_id":"acct_123"`)
			assert.Contains(t, recorder.Body.String(), `"channel_name":"Codex Channel"`)
		}
	})
}
