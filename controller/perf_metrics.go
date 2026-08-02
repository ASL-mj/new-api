package controller

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

var (
	queryModelPerformance        = perfmetrics.Query
	queryModelPerformanceSummary = perfmetrics.QuerySummaryAll
)

func GetModelPerformanceSummary(c *gin.Context) {
	hours, ok := parsePerformanceHours(c.Query("hours"))
	if !ok {
		modelPerformanceError(c, http.StatusBadRequest, i18n.MsgPerfMetricsHoursInvalid)
		return
	}

	result, err := queryModelPerformanceSummary(hours, activePerformanceGroups())
	if err != nil {
		modelPerformanceError(c, http.StatusInternalServerError, i18n.MsgPerfMetricsSummaryFailed)
		return
	}
	if result.Models == nil {
		result.Models = []perfmetrics.ModelSummary{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetModelPerformance(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		modelPerformanceError(c, http.StatusBadRequest, i18n.MsgPerfMetricsModelRequired)
		return
	}

	hours, ok := parsePerformanceHours(c.Query("hours"))
	if !ok {
		modelPerformanceError(c, http.StatusBadRequest, i18n.MsgPerfMetricsHoursInvalid)
		return
	}

	result, err := queryModelPerformance(perfmetrics.QueryParams{
		Model:         modelName,
		Group:         strings.TrimSpace(c.Query("group")),
		Hours:         hours,
		AllowedGroups: activePerformanceGroups(),
	})
	if err != nil {
		modelPerformanceError(c, http.StatusInternalServerError, i18n.MsgPerfMetricsQueryFailed)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func modelPerformanceError(c *gin.Context, status int, key string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": common.TranslateMessage(c, key),
	})
}

func parsePerformanceHours(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 24, true
	}

	hours, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	switch hours {
	case 1, 24, 168:
		return hours, true
	default:
		return 0, false
	}
}

func activePerformanceGroups() []string {
	groupRatio := ratio_setting.GetGroupRatioCopy()
	groups := make([]string, 0, len(groupRatio)+1)
	for group := range groupRatio {
		groups = append(groups, group)
	}

	// auto is a token-level virtual group and is not part of GroupRatio.
	if _, exists := groupRatio["auto"]; !exists {
		groups = append(groups, "auto")
	}
	sort.Strings(groups)
	return groups
}
