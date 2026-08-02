package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withModelPerformanceQuery(t *testing.T, query func(perfmetrics.QueryParams) (perfmetrics.QueryResult, error)) {
	t.Helper()
	original := queryModelPerformance
	queryModelPerformance = query
	t.Cleanup(func() {
		queryModelPerformance = original
	})
}

func withModelPerformanceSummaryQuery(t *testing.T, query func(int, []string) (perfmetrics.SummaryAllResult, error)) {
	t.Helper()
	original := queryModelPerformanceSummary
	queryModelPerformanceSummary = query
	t.Cleanup(func() {
		queryModelPerformanceSummary = original
	})
}

func withPerformanceGroupRatios(t *testing.T, ratios string) {
	t.Helper()
	original := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(ratios))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(original))
	})
}

func performModelPerformanceRequest(t *testing.T, target string) *httptest.ResponseRecorder {
	return performModelPerformanceRequestWithLanguage(t, target, "en")
}

func performModelPerformanceRequestWithLanguage(t *testing.T, target, language string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ctx.Request.Header.Set("Accept-Language", language)
	GetModelPerformance(ctx)
	return recorder
}

func TestGetModelPerformanceErrorsAreLocalized(t *testing.T) {
	recorder := performModelPerformanceRequestWithLanguage(t, "/api/perf-metrics", "zh-TW")
	response := decodeModelPerformanceResponse(t, recorder)
	require.Equal(t, "必須提供模型名稱", response["message"])
}

func performModelPerformanceSummaryRequest(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	GetModelPerformanceSummary(ctx)
	return recorder
}

func decodeModelPerformanceResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	response := make(map[string]any)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestGetModelPerformanceRequiresModel(t *testing.T) {
	called := false
	withModelPerformanceQuery(t, func(perfmetrics.QueryParams) (perfmetrics.QueryResult, error) {
		called = true
		return perfmetrics.QueryResult{}, nil
	})

	recorder := performModelPerformanceRequest(t, "/api/perf-metrics?model=%20%20")
	response := decodeModelPerformanceResponse(t, recorder)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.False(t, response["success"].(bool))
	require.Equal(t, "model is required", response["message"])
	require.False(t, called)
}

func TestGetModelPerformanceRejectsInvalidHours(t *testing.T) {
	for _, hours := range []string{"0", "2", "169", "abc"} {
		t.Run(hours, func(t *testing.T) {
			called := false
			withModelPerformanceQuery(t, func(perfmetrics.QueryParams) (perfmetrics.QueryResult, error) {
				called = true
				return perfmetrics.QueryResult{}, nil
			})

			recorder := performModelPerformanceRequest(t, "/api/perf-metrics?model=gpt-4.1&hours="+hours)
			response := decodeModelPerformanceResponse(t, recorder)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.False(t, response["success"].(bool))
			require.False(t, called)
		})
	}
}

func TestGetModelPerformanceDefaultsHoursAndFiltersActiveGroups(t *testing.T) {
	withPerformanceGroupRatios(t, `{"active":1,"priority":2}`)

	var received perfmetrics.QueryParams
	withModelPerformanceQuery(t, func(params perfmetrics.QueryParams) (perfmetrics.QueryResult, error) {
		received = params
		return perfmetrics.QueryResult{
			ModelName:    params.Model,
			Hours:        params.Hours,
			SeriesSchema: "test-schema",
			Overall: perfmetrics.AggregateResult{
				Series: []perfmetrics.BucketPoint{},
			},
			Groups: []perfmetrics.GroupResult{},
		}, nil
	})

	recorder := performModelPerformanceRequest(t, "/api/perf-metrics?model=gpt-4.1&group=active")
	response := decodeModelPerformanceResponse(t, recorder)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, response["success"].(bool))
	require.Equal(t, "gpt-4.1", received.Model)
	require.Equal(t, "active", received.Group)
	require.Equal(t, 24, received.Hours)
	require.Equal(t, []string{"active", "auto", "priority"}, received.AllowedGroups)

	data := response["data"].(map[string]any)
	require.Equal(t, float64(24), data["hours"])
	require.Equal(t, "test-schema", data["series_schema"])
	require.Equal(t, []any{}, data["groups"])
}

func TestGetModelPerformanceReturnsStableEmptyData(t *testing.T) {
	withModelPerformanceQuery(t, func(params perfmetrics.QueryParams) (perfmetrics.QueryResult, error) {
		return perfmetrics.QueryResult{
			ModelName:    params.Model,
			Hours:        params.Hours,
			SeriesSchema: "empty-schema",
			Overall: perfmetrics.AggregateResult{
				Series: []perfmetrics.BucketPoint{},
			},
			Groups: []perfmetrics.GroupResult{},
		}, nil
	})

	recorder := performModelPerformanceRequest(t, "/api/perf-metrics?model=empty-model")
	response := decodeModelPerformanceResponse(t, recorder)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, response["success"].(bool))

	data := response["data"].(map[string]any)
	require.Equal(t, "empty-model", data["model_name"])
	require.Equal(t, float64(24), data["hours"])
	require.Equal(t, "empty-schema", data["series_schema"])
	require.Equal(t, []any{}, data["groups"])
	params := data["overall"].(map[string]any)
	require.Equal(t, []any{}, params["series"])
}

func TestGetModelPerformanceReturnsInternalErrorWithoutLeakingQueryError(t *testing.T) {
	withModelPerformanceQuery(t, func(perfmetrics.QueryParams) (perfmetrics.QueryResult, error) {
		return perfmetrics.QueryResult{}, errors.New("database connection details must not be exposed")
	})

	recorder := performModelPerformanceRequest(t, "/api/perf-metrics?model=gpt-4.1&hours=1")
	response := decodeModelPerformanceResponse(t, recorder)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.False(t, response["success"].(bool))
	require.Equal(t, "failed to query model performance", response["message"])
}

func TestGetModelPerformanceSummaryDefaultsHoursAndFiltersActiveGroups(t *testing.T) {
	withPerformanceGroupRatios(t, `{"default":1,"vip":2}`)

	var receivedHours int
	var receivedGroups []string
	withModelPerformanceSummaryQuery(t, func(hours int, groups []string) (perfmetrics.SummaryAllResult, error) {
		receivedHours = hours
		receivedGroups = groups
		return perfmetrics.SummaryAllResult{Models: []perfmetrics.ModelSummary{}}, nil
	})

	recorder := performModelPerformanceSummaryRequest(t, "/api/perf-metrics/summary")
	response := decodeModelPerformanceResponse(t, recorder)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, response["success"].(bool))
	assert.Equal(t, 24, receivedHours)
	assert.Equal(t, []string{"auto", "default", "vip"}, receivedGroups)
	data := response["data"].(map[string]any)
	assert.Equal(t, []any{}, data["models"])
}

func TestGetModelPerformanceSummaryRejectsInvalidHours(t *testing.T) {
	called := false
	withModelPerformanceSummaryQuery(t, func(int, []string) (perfmetrics.SummaryAllResult, error) {
		called = true
		return perfmetrics.SummaryAllResult{}, nil
	})

	recorder := performModelPerformanceSummaryRequest(t, "/api/perf-metrics/summary?hours=2")
	response := decodeModelPerformanceResponse(t, recorder)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.False(t, response["success"].(bool))
	require.False(t, called)
}

func TestGetModelPerformanceSummaryReturnsStableEmptyData(t *testing.T) {
	withModelPerformanceSummaryQuery(t, func(int, []string) (perfmetrics.SummaryAllResult, error) {
		return perfmetrics.SummaryAllResult{}, nil
	})

	recorder := performModelPerformanceSummaryRequest(t, "/api/perf-metrics/summary?hours=24")
	response := decodeModelPerformanceResponse(t, recorder)

	require.Equal(t, http.StatusOK, recorder.Code)
	data := response["data"].(map[string]any)
	assert.Equal(t, []any{}, data["models"])
}

func TestGetModelPerformanceSummaryHidesQueryError(t *testing.T) {
	withModelPerformanceSummaryQuery(t, func(int, []string) (perfmetrics.SummaryAllResult, error) {
		return perfmetrics.SummaryAllResult{}, errors.New("database connection details must not be exposed")
	})

	recorder := performModelPerformanceSummaryRequest(t, "/api/perf-metrics/summary?hours=1")
	response := decodeModelPerformanceResponse(t, recorder)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.False(t, response["success"].(bool))
	require.Equal(t, "failed to query model performance summary", response["message"])
}
