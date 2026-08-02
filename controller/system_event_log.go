package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetSystemEventLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	start, err := optionalTimestamp(c.Query("start_timestamp"), 0)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgSystemEventInvalidStartTimestamp)
		return
	}
	end, err := optionalTimestamp(c.Query("end_timestamp"), 0)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgSystemEventInvalidEndTimestamp)
		return
	}
	if start > 0 && end > 0 && end < start {
		common.ApiErrorI18n(c, i18n.MsgSystemEventEndBeforeStart)
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
	for index := range rows {
		rows[index] = localizeSystemEventLog(c, rows[index])
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

func localizeSystemEventLog(c *gin.Context, row model.SystemEventLog) model.SystemEventLog {
	if row.MessageKey == "" {
		return row
	}
	args := map[string]any{}
	if row.Extra != "" {
		_ = common.Unmarshal([]byte(row.Extra), &args)
	}
	for key, value := range args {
		args[systemEventTemplateArgName(key)] = value
	}
	args["RequestId"] = row.RequestId
	args["ChannelId"] = row.ChannelId
	args["ModelName"] = row.ModelName
	args["Group"] = row.Group
	args["StatusCode"] = row.StatusCode
	args["LatencyMs"] = row.LatencyMs

	translated := common.TranslateMessage(c, row.MessageKey, args)
	if translated != "" && translated != row.MessageKey {
		row.Message = translated
	}
	return row
}

func systemEventTemplateArgName(value string) string {
	parts := strings.Split(value, "_")
	for index := range parts {
		if parts[index] == "" {
			continue
		}
		parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
	}
	return strings.Join(parts, "")
}
