package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxChannelUsageBatchIDs = 200

type updateChannelKeyQuotaLimitRequest struct {
	QuotaLimit int64 `json:"quota_limit"`
}

func GetChannelUsageBatch(c *gin.Context) {
	ids, err := parseChannelUsageIDs(c.Query("ids"))
	if err != nil {
		channelUsageError(c, err)
		return
	}
	today, start30d, err := model.GetChannelUsageDateRange(time.Now(), 30)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	stats, err := model.GetChannelUsageStatsBatch(ids, today, start30d)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": stats})
}

func ResetChannelQuotaUsage(c *gin.Context) {
	channel, ok := getChannelForUsageOperation(c)
	if !ok {
		return
	}
	if err := channel.ResetQuotaLimitUsage(common.GetTimestamp()); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "渠道限额用量已重置", "data": channel})
}

func GetChannelKeyUsageList(c *gin.Context) {
	channel, ok := getChannelForUsageOperation(c)
	if !ok {
		return
	}
	usages, err := model.GetChannelKeyUsages(channel)
	if err != nil {
		channelUsageError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": usages})
}

func ResetChannelKeyQuotaUsage(c *gin.Context) {
	channelID, fingerprint, ok := getChannelKeyUsageIdentity(c)
	if !ok {
		return
	}
	if err := model.ResetChannelKeyQuotaUsage(channelID, fingerprint, common.GetTimestamp()); err != nil {
		channelUsageError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "密钥限额用量已重置"})
}

func UpdateChannelKeyQuotaLimit(c *gin.Context) {
	channelID, fingerprint, ok := getChannelKeyUsageIdentity(c)
	if !ok {
		return
	}
	request := updateChannelKeyQuotaLimitRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		channelUsageError(c, errors.New("参数错误"))
		return
	}
	if request.QuotaLimit < 0 {
		channelUsageError(c, errors.New("密钥限额不能小于 0"))
		return
	}
	if err := model.UpdateChannelKeyQuotaLimit(channelID, fingerprint, request.QuotaLimit); err != nil {
		channelUsageError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "密钥限额已更新"})
}

func parseChannelUsageIDs(raw string) ([]int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	if len(parts) == 1 && strings.TrimSpace(parts[0]) == "" {
		return nil, errors.New("渠道 ID 不能为空")
	}
	if len(parts) > maxChannelUsageBatchIDs {
		return nil, errors.New("单次最多查询 200 个渠道")
	}
	ids := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return nil, errors.New("渠道 ID 格式错误")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func getChannelForUsageOperation(c *gin.Context) (*model.Channel, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		channelUsageError(c, errors.New("渠道 ID 格式错误"))
		return nil, false
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		channelUsageError(c, err)
		return nil, false
	}
	return channel, true
}

func getChannelKeyUsageIdentity(c *gin.Context) (int, string, bool) {
	channel, ok := getChannelForUsageOperation(c)
	if !ok {
		return 0, "", false
	}
	if !channel.ChannelInfo.IsMultiKey {
		channelUsageError(c, errors.New("渠道不是多密钥渠道"))
		return 0, "", false
	}
	fingerprint := strings.TrimSpace(c.Param("fingerprint"))
	if len(fingerprint) != 64 {
		channelUsageError(c, errors.New("密钥指纹格式错误"))
		return 0, "", false
	}
	usages, err := model.GetChannelKeyUsages(channel)
	if err != nil {
		channelUsageError(c, err)
		return 0, "", false
	}
	found := false
	for _, usage := range usages {
		if usage.KeyFingerprint == fingerprint {
			found = true
			break
		}
	}
	if !found {
		channelUsageError(c, gorm.ErrRecordNotFound)
		return 0, "", false
	}
	return channel.Id, fingerprint, true
}

func channelUsageError(c *gin.Context, err error) {
	message := err.Error()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		message = "记录不存在"
	}
	c.JSON(http.StatusOK, gin.H{"success": false, "message": message})
}
