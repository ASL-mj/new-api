package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetSystemEventLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	start, err := optionalTimestamp(c.Query("start_timestamp"), 0)
	if err != nil {
		common.ApiErrorMsg(c, "invalid start_timestamp")
		return
	}
	end, err := optionalTimestamp(c.Query("end_timestamp"), 0)
	if err != nil {
		common.ApiErrorMsg(c, "invalid end_timestamp")
		return
	}
	if start > 0 && end > 0 && end < start {
		common.ApiErrorMsg(c, "end_timestamp must not be before start_timestamp")
		return
	}
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
