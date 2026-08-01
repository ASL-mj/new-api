package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetSystemEventLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	start, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	rows, total, err := model.GetSystemEventLogs(model.SystemEventLogQuery{
		StartTimestamp: start, EndTimestamp: end, Level: c.Query("level"), Component: c.Query("component"),
		RequestId: c.Query("request_id"),
	}, pageInfo.Page, pageInfo.PageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}
