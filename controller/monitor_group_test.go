package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performMonitorGroupRequest(t *testing.T, method, target, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	return recorder
}

func decodeMonitorGroupResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	response := make(map[string]any)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestMonitorGroupIntervalSecondsBounds(t *testing.T) {
	for _, interval := range []int{15, 60, 3600} {
		body := `{"name":"Bounds","key":"bounds","primary_model":"gpt-5.4","channel_ids":[1],"enabled":true,"user_visible":true,"interval_seconds":` + strconv.Itoa(interval) + `,"timeout_seconds":30,"degraded_ms":3000}`
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/monitor_group/", bytes.NewBufferString(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		group, _, err := bindMonitorGroupRequest(ctx, false)
		require.NoError(t, err)
		assert.Equal(t, interval, group.IntervalSeconds)
	}

	for _, interval := range []int{-1, 0, 14, 3601} {
		body := `{"name":"Bounds","key":"bounds","primary_model":"gpt-5.4","channel_ids":[1],"enabled":true,"user_visible":true,"interval_seconds":` + strconv.Itoa(interval) + `,"timeout_seconds":30,"degraded_ms":3000}`
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/monitor_group/", bytes.NewBufferString(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		_, _, err := bindMonitorGroupRequest(ctx, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "15-3600 秒")
	}
}

func TestCreateAndListMonitorGroupsDoNotExposeChannelKey(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Type: 1, Key: "test-channel-secret", Name: "Visible Channel", Models: "gpt-5.4,gpt-5.4-mini"}
	require.NoError(t, model.DB.Create(channel).Error)

	create := performMonitorGroupRequest(t, http.MethodPost, "/api/monitor_group/", `{
		"name":"Core","key":"core","primary_model":"gpt-5.4","extra_models":["gpt-5.4-mini"],
		"channel_ids":[1],"enabled":true,"user_visible":true,"interval_seconds":600,"timeout_seconds":30,"degraded_ms":3000
	}`, CreateMonitorGroup)
	created := decodeMonitorGroupResponse(t, create)
	require.True(t, created["success"].(bool))
	assert.NotContains(t, create.Body.String(), "test-channel-secret")
	assert.Contains(t, create.Body.String(), "Visible Channel")

	list := performMonitorGroupRequest(t, http.MethodGet, "/api/monitor_group/?p=1&page_size=20", "", GetMonitorGroups)
	listed := decodeMonitorGroupResponse(t, list)
	require.True(t, listed["success"].(bool))
	assert.NotContains(t, list.Body.String(), "test-channel-secret")
	assert.Contains(t, list.Body.String(), "Visible Channel")

	options := performMonitorGroupRequest(t, http.MethodGet, "/api/monitor_group/channels", "", GetMonitorGroupChannelOptions)
	optionResponse := decodeMonitorGroupResponse(t, options)
	require.True(t, optionResponse["success"].(bool))
	assert.NotContains(t, options.Body.String(), "test-channel-secret")
	assert.Contains(t, options.Body.String(), "Visible Channel")
	assert.Contains(t, options.Body.String(), `"type_name":"OpenAI"`)
}

func TestCreateMonitorGroupRejectsInvalidKeyAndUpdateKeepsKeyImmutable(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Type: 1, Key: "test-channel-secret", Name: "Visible Channel", Models: "gpt-5.4"}
	require.NoError(t, model.DB.Create(channel).Error)

	invalid := performMonitorGroupRequest(t, http.MethodPost, "/api/monitor_group/", `{
		"name":"Bad","key":"UPPER CASE","primary_model":"gpt-5.4","channel_ids":[1],
		"enabled":true,"user_visible":true,"interval_seconds":600,"timeout_seconds":30,"degraded_ms":3000
	}`, CreateMonitorGroup)
	invalidResponse := decodeMonitorGroupResponse(t, invalid)
	assert.False(t, invalidResponse["success"].(bool))

	group := &model.MonitorGroup{Name: "Stable", Key: "stable", PrimaryModel: "gpt-5.4", Enabled: true, UserVisible: true, IntervalSeconds: 600, TimeoutSeconds: 30, DegradedMs: 3000}
	require.NoError(t, model.CreateMonitorGroup(group, []int{channel.Id}))

	update := performMonitorGroupRequest(t, http.MethodPut, "/api/monitor_group/", `{
		"id":1,"name":"Stable","key":"changed","primary_model":"gpt-5.4","channel_ids":[1],
		"enabled":true,"user_visible":true,"interval_seconds":600,"timeout_seconds":30,"degraded_ms":3000
	}`, UpdateMonitorGroup)
	updated := decodeMonitorGroupResponse(t, update)
	assert.False(t, updated["success"].(bool))

	persisted, err := model.GetMonitorGroupById(group.Id)
	require.NoError(t, err)
	assert.Equal(t, "stable", persisted.Key)
}

func TestUserMonitorStatusRedactsChannelAndUpstreamDetails(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channel := &model.Channel{Type: 1, Key: "test-channel-secret", Name: "Internal Channel", Models: "gpt-5.4"}
	require.NoError(t, model.DB.Create(channel).Error)
	publicGroup := &model.MonitorGroup{Name: "Public Group", Key: "public", PrimaryModel: "gpt-5.4", Enabled: true, UserVisible: true, IntervalSeconds: 600, TimeoutSeconds: 30, DegradedMs: 3000}
	hiddenGroup := &model.MonitorGroup{Name: "Hidden Group", Key: "hidden", PrimaryModel: "gpt-5.4", Enabled: true, UserVisible: false, IntervalSeconds: 600, TimeoutSeconds: 30, DegradedMs: 3000}
	require.NoError(t, model.CreateMonitorGroup(publicGroup, []int{channel.Id}))
	require.NoError(t, model.CreateMonitorGroup(hiddenGroup, []int{channel.Id}))
	now := time.Now().Unix()
	ping := int64(21)
	require.NoError(t, model.InsertMonitorChecks([]*model.MonitorCheck{
		{MonitorGroupId: publicGroup.Id, ChannelId: channel.Id, ModelName: "gpt-5.4", Status: model.MonitorCheckStatusOperational, LatencyMs: 120, PingLatencyMs: &ping, CheckedAt: now - 60},
		{MonitorGroupId: publicGroup.Id, ChannelId: channel.Id, ModelName: "gpt-5.4", Status: model.MonitorCheckStatusFailed, ErrorCode: "upstream_500", ErrorMessage: "upstream secret detail", CheckedAt: now - 10},
	}))

	recorder := performMonitorGroupRequest(t, http.MethodGet, "/api/monitor_status/", "", GetMonitorStatus)
	response := decodeMonitorGroupResponse(t, recorder)
	require.True(t, response["success"].(bool))
	assert.Contains(t, recorder.Body.String(), "Public Group")
	assert.NotContains(t, recorder.Body.String(), "Hidden Group")
	assert.NotContains(t, recorder.Body.String(), "channel_id")
	assert.NotContains(t, recorder.Body.String(), "channel_name")
	assert.NotContains(t, recorder.Body.String(), "test-channel-secret")
	assert.NotContains(t, recorder.Body.String(), "error_code")
	assert.NotContains(t, recorder.Body.String(), "error_message")
	assert.NotContains(t, recorder.Body.String(), "upstream secret detail")
	assert.NotContains(t, recorder.Body.String(), "target_count")
	assert.NotContains(t, recorder.Body.String(), "model_count")
	assert.Contains(t, recorder.Body.String(), `"primary_model":"gpt-5.4"`)
	assert.Contains(t, recorder.Body.String(), `"channel_types":["OpenAI"]`)
	assert.Contains(t, recorder.Body.String(), `"current_ping_latency_ms":21`)

	detailRecorder := httptest.NewRecorder()
	detailContext, _ := gin.CreateTestContext(detailRecorder)
	detailContext.Request = httptest.NewRequest(http.MethodGet, "/api/monitor_status/"+strconv.Itoa(publicGroup.Id)+"?days=7", nil)
	detailContext.AddParam("id", strconv.Itoa(publicGroup.Id))
	GetMonitorStatusGroup(detailContext)
	detail := decodeMonitorGroupResponse(t, detailRecorder)
	require.True(t, detail["success"].(bool))
	assert.Contains(t, detailRecorder.Body.String(), "availability_history")
	assert.Contains(t, detailRecorder.Body.String(), "availability_days")
	assert.NotContains(t, detailRecorder.Body.String(), "channel_id")
	assert.NotContains(t, detailRecorder.Body.String(), "upstream secret detail")
}

func TestCreateMonitorGroupAcceptsOnlyCommonChannelModels(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channelA := &model.Channel{Type: 1, Key: "a", Name: "A", Models: "gpt-5.4,gpt-5.4-mini"}
	channelB := &model.Channel{Type: 1, Key: "b", Name: "B", Models: "gpt-5.4,gpt-5.5"}
	require.NoError(t, model.DB.Create(channelA).Error)
	require.NoError(t, model.DB.Create(channelB).Error)

	accepted := performMonitorGroupRequest(t, http.MethodPost, "/api/monitor_group/", `{
		"name":"Common","key":"common","primary_model":"gpt-5.4","channel_ids":[1,2],
		"enabled":true,"user_visible":true,"interval_seconds":600,"timeout_seconds":30,"degraded_ms":3000
	}`, CreateMonitorGroup)
	acceptedResponse := decodeMonitorGroupResponse(t, accepted)
	require.True(t, acceptedResponse["success"].(bool), accepted.Body.String())

	rejected := performMonitorGroupRequest(t, http.MethodPost, "/api/monitor_group/", `{
		"name":"Unsupported","key":"unsupported","primary_model":"gpt-5.4-mini","channel_ids":[1,2],
		"enabled":true,"user_visible":true,"interval_seconds":600,"timeout_seconds":30,"degraded_ms":3000
	}`, CreateMonitorGroup)
	rejectedResponse := decodeMonitorGroupResponse(t, rejected)
	assert.False(t, rejectedResponse["success"].(bool))
	assert.Contains(t, rejected.Body.String(), "共同支持")
}

func TestUpdateMonitorGroupRejectsModelOutsideIntersection(t *testing.T) {
	prepareMonitorRunnerTables(t)
	channelA := &model.Channel{Type: 1, Key: "a", Name: "A", Models: "gpt-5.4,gpt-5.4-mini"}
	channelB := &model.Channel{Type: 14, Key: "b", Name: "B", Models: "gpt-5.4,gpt-5.5"}
	require.NoError(t, model.DB.Create(channelA).Error)
	require.NoError(t, model.DB.Create(channelB).Error)
	group := &model.MonitorGroup{Name: "Common", Key: "common", PrimaryModel: "gpt-5.4", Enabled: true, UserVisible: true, IntervalSeconds: 600, TimeoutSeconds: 30, DegradedMs: 3000}
	require.NoError(t, model.CreateMonitorGroup(group, []int{channelA.Id, channelB.Id}))

	recorder := performMonitorGroupRequest(t, http.MethodPut, "/api/monitor_group/", `{
		"id":1,"name":"Common","key":"common","primary_model":"gpt-5.4","extra_models":["gpt-5.5"],"channel_ids":[1,2],
		"enabled":true,"user_visible":true,"interval_seconds":600,"timeout_seconds":30,"degraded_ms":3000
	}`, UpdateMonitorGroup)
	response := decodeMonitorGroupResponse(t, recorder)
	assert.False(t, response["success"].(bool))
	assert.Contains(t, recorder.Body.String(), "共同支持")

	list := performMonitorGroupRequest(t, http.MethodGet, "/api/monitor_group/?p=1&page_size=20", "", GetMonitorGroups)
	assert.Contains(t, list.Body.String(), `"channel_types":["OpenAI","Anthropic"]`)
	assert.Contains(t, list.Body.String(), `"running":false`)
}
