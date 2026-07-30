package controller

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

var queryModelPerformance = perfmetrics.Query

func GetModelPerformance(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours, ok := parsePerformanceHours(c.Query("hours"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "hours must be one of: 1, 24, 168",
		})
		return
	}

	result, err := queryModelPerformance(perfmetrics.QueryParams{
		Model:         modelName,
		Group:         strings.TrimSpace(c.Query("group")),
		Hours:         hours,
		AllowedGroups: activePerformanceGroups(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to query model performance",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
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
