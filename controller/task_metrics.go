package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetTaskMetrics(c *gin.Context) {
	snapshot, err := service.GetTaskMetricsSnapshot(c.Request.Context())
	if err != nil {
		common.ApiError(c, errors.New("Task metrics are temporarily unavailable"))
		return
	}
	common.ApiSuccess(c, snapshot)
}
