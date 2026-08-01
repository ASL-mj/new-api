package controller

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

var monitorGroupKeyPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

type MonitorGroupRequest struct {
	Id              int      `json:"id"`
	Name            string   `json:"name" binding:"required"`
	Key             string   `json:"key" binding:"required"`
	Description     string   `json:"description"`
	PrimaryModel    string   `json:"primary_model" binding:"required"`
	ExtraModels     []string `json:"extra_models"`
	ChannelIds      []int    `json:"channel_ids" binding:"required"`
	Enabled         bool     `json:"enabled"`
	UserVisible     bool     `json:"user_visible"`
	IntervalSeconds int      `json:"interval_seconds"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	DegradedMs      int      `json:"degraded_ms"`
}

type MonitorGroupTargetResponse struct {
	Id          int    `json:"id"`
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Models      string `json:"models"`
	Enabled     bool   `json:"enabled"`
}

// MonitorGroupChannelOption intentionally contains only the fields needed by
// the monitor configuration form. In particular, it must never expose a
// channel key or upstream credential to the browser.
type MonitorGroupChannelOption struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	TypeName string `json:"type_name"`
	Status   int    `json:"status"`
	Group    string `json:"group"`
	Models   string `json:"models"`
}

type AdminMonitorGroupResponse struct {
	model.MonitorGroup
	Targets      []MonitorGroupTargetResponse `json:"targets"`
	ChannelTypes []string                     `json:"channel_types"`
	Running      bool                         `json:"running"`
}

func GetMonitorGroups(c *gin.Context) {
	status, err := parseMonitorGroupStatus(c.Query("status"))
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	pageInfo := common.GetPageQuery(c)
	groups, total, err := model.GetMonitorGroups(pageInfo, c.Query("search"), status)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items, err := makeAdminMonitorGroupResponses(groups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func GetMonitorGroupChannelOptions(c *gin.Context) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	options := make([]MonitorGroupChannelOption, 0, len(channels))
	for _, channel := range channels {
		options = append(options, MonitorGroupChannelOption{
			Id: channel.Id, Name: channel.Name, Type: channel.Type, TypeName: constant.GetChannelTypeName(channel.Type),
			Status: channel.Status, Group: channel.Group, Models: channel.Models,
		})
	}
	common.ApiSuccess(c, options)
}

func GetMonitorGroup(c *gin.Context) {
	id, err := parseMonitorGroupId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	group, err := model.GetMonitorGroupById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items, err := makeAdminMonitorGroupResponses([]*model.MonitorGroup{group})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items[0])
}

func CreateMonitorGroup(c *gin.Context) {
	group, channelIds, err := bindMonitorGroupRequest(c, false)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	channels, err := ensureMonitorGroupChannelsExist(channelIds)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	models := getMonitorTargetModels(group, nil)
	allowedModels, err := intersectMonitorChannelModels(channels)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := validateMonitorGroupModels(group.PrimaryModel, models[1:], allowedModels); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.CreateMonitorGroup(group, channelIds); err != nil {
		common.ApiError(c, err)
		return
	}
	items, err := makeAdminMonitorGroupResponses([]*model.MonitorGroup{group})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items[0])
}

func UpdateMonitorGroup(c *gin.Context) {
	group, channelIds, err := bindMonitorGroupRequest(c, true)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	channels, err := ensureMonitorGroupChannelsExist(channelIds)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	models := getMonitorTargetModels(group, nil)
	allowedModels, err := intersectMonitorChannelModels(channels)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := validateMonitorGroupModels(group.PrimaryModel, models[1:], allowedModels); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.UpdateMonitorGroup(group, channelIds); err != nil {
		common.ApiError(c, err)
		return
	}
	items, err := makeAdminMonitorGroupResponses([]*model.MonitorGroup{group})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items[0])
}

func DeleteMonitorGroup(c *gin.Context) {
	id, err := parseMonitorGroupId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.DeleteMonitorGroup(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func RunMonitorGroup(c *gin.Context) {
	id, err := parseMonitorGroupId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := RunMonitorGroupNow(id); err != nil {
		if errors.Is(err, errMonitorGroupRunning) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "该监控分组正在检测中"})
			return
		}
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "监控分组检测已开始"})
}

func GetMonitorGroupHistory(c *gin.Context) {
	id, err := parseMonitorGroupId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	limit, err := parseMonitorGroupBoundedInt(c.Query("limit"), 60, 1, 1000)
	if err != nil {
		common.ApiErrorMsg(c, "invalid limit")
		return
	}
	days, err := parseMonitorGroupBoundedInt(c.Query("days"), 30, 1, 30)
	if err != nil {
		common.ApiErrorMsg(c, "invalid days")
		return
	}
	checks, err := model.GetMonitorTimeline(id, strings.TrimSpace(c.Query("model")), limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	availability, err := model.GetMonitorAvailability(id, days)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"checks": checks, "availability": availability})
}

func bindMonitorGroupRequest(c *gin.Context, requireId bool) (*model.MonitorGroup, []int, error) {
	request := &MonitorGroupRequest{}
	if err := c.ShouldBindJSON(request); err != nil {
		return nil, nil, err
	}
	if requireId && request.Id <= 0 {
		return nil, nil, errors.New("监控分组 ID 无效")
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Key = strings.TrimSpace(request.Key)
	request.Description = strings.TrimSpace(request.Description)
	request.PrimaryModel = strings.TrimSpace(request.PrimaryModel)
	if request.Name == "" || len(request.Name) > 100 {
		return nil, nil, errors.New("监控分组名称长度必须为 1-100")
	}
	if !monitorGroupKeyPattern.MatchString(request.Key) || len(request.Key) > 64 {
		return nil, nil, errors.New("监控分组标识只允许小写字母、数字、下划线和连字符")
	}
	if len(request.Description) > 255 || request.PrimaryModel == "" || len(request.PrimaryModel) > 128 {
		return nil, nil, errors.New("监控模型或描述无效")
	}
	if request.IntervalSeconds < 15 || request.IntervalSeconds > 3600 {
		return nil, nil, errors.New("检测间隔必须在 15-3600 秒之间")
	}
	if request.TimeoutSeconds < 5 || request.TimeoutSeconds > 120 {
		return nil, nil, errors.New("超时时间必须在 5-120 秒之间")
	}
	if request.DegradedMs <= 0 || request.DegradedMs > 300000 {
		return nil, nil, errors.New("降级延迟阈值必须在 1-300000 毫秒之间")
	}

	channelIds := uniqueMonitorGroupChannelIds(request.ChannelIds)
	if len(channelIds) == 0 {
		return nil, nil, errors.New("至少选择一个渠道")
	}
	extraModels := normalizeMonitorGroupModels(request.ExtraModels)
	extraModelsJSON, err := common.Marshal(extraModels)
	if err != nil {
		return nil, nil, err
	}
	group := &model.MonitorGroup{
		Id:              request.Id,
		Name:            request.Name,
		Key:             request.Key,
		Description:     request.Description,
		PrimaryModel:    request.PrimaryModel,
		ExtraModels:     string(extraModelsJSON),
		Enabled:         request.Enabled,
		UserVisible:     request.UserVisible,
		IntervalSeconds: request.IntervalSeconds,
		TimeoutSeconds:  request.TimeoutSeconds,
		DegradedMs:      request.DegradedMs,
	}
	return group, channelIds, nil
}

func makeAdminMonitorGroupResponses(groups []*model.MonitorGroup) ([]AdminMonitorGroupResponse, error) {
	responses := make([]AdminMonitorGroupResponse, len(groups))
	if len(groups) == 0 {
		return responses, nil
	}
	groupIds := make([]int, 0, len(groups))
	for _, group := range groups {
		groupIds = append(groupIds, group.Id)
	}
	targets, err := model.GetMonitorGroupTargetsByGroupIds(groupIds)
	if err != nil {
		return nil, err
	}
	channelIds := make([]int, 0, len(targets))
	for _, target := range targets {
		channelIds = append(channelIds, target.ChannelId)
	}
	channels, err := model.GetChannelsByIds(uniqueMonitorGroupChannelIds(channelIds))
	if err != nil {
		return nil, err
	}
	channelNames := make(map[int]string, len(channels))
	channelById := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		channelNames[channel.Id] = channel.Name
		channelById[channel.Id] = channel
	}
	targetsByGroup := make(map[int][]MonitorGroupTargetResponse, len(groups))
	channelsByGroup := make(map[int][]*model.Channel, len(groups))
	for _, target := range targets {
		targetsByGroup[target.MonitorGroupId] = append(targetsByGroup[target.MonitorGroupId], MonitorGroupTargetResponse{
			Id:          target.Id,
			ChannelId:   target.ChannelId,
			ChannelName: channelNames[target.ChannelId],
			Models:      target.Models,
			Enabled:     target.Enabled,
		})
		if channel := channelById[target.ChannelId]; channel != nil {
			channelsByGroup[target.MonitorGroupId] = append(channelsByGroup[target.MonitorGroupId], channel)
		}
	}
	for i, group := range groups {
		responses[i] = AdminMonitorGroupResponse{
			MonitorGroup: *group,
			Targets:      targetsByGroup[group.Id],
			ChannelTypes: monitorChannelTypeNames(channelsByGroup[group.Id]),
			Running:      isMonitorGroupRunning(group.Id),
		}
		if responses[i].Targets == nil {
			responses[i].Targets = []MonitorGroupTargetResponse{}
		}
		if responses[i].ChannelTypes == nil {
			responses[i].ChannelTypes = []string{}
		}
	}
	return responses, nil
}

func ensureMonitorGroupChannelsExist(channelIds []int) ([]*model.Channel, error) {
	channels, err := model.GetChannelsByIds(channelIds)
	if err != nil {
		return nil, err
	}
	if len(channels) != len(channelIds) {
		return nil, errors.New("选择的渠道不存在或已被删除")
	}
	channelById := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		channelById[channel.Id] = channel
	}
	ordered := make([]*model.Channel, 0, len(channelIds))
	for _, channelId := range channelIds {
		ordered = append(ordered, channelById[channelId])
	}
	return ordered, nil
}

func parseMonitorChannelModels(value string) []string {
	return normalizeMonitorGroupModels(strings.Split(value, ","))
}

func intersectMonitorChannelModels(channels []*model.Channel) ([]string, error) {
	if len(channels) == 0 {
		return nil, errors.New("至少选择一个渠道")
	}
	intersection := parseMonitorChannelModels(channels[0].Models)
	if len(intersection) == 0 {
		return nil, errors.New("所选渠道没有可用于探测的模型")
	}
	for _, channel := range channels[1:] {
		models := parseMonitorChannelModels(channel.Models)
		if len(models) == 0 {
			return nil, errors.New("所选渠道没有可用于探测的模型")
		}
		available := make(map[string]struct{}, len(models))
		for _, modelName := range models {
			available[modelName] = struct{}{}
		}
		filtered := intersection[:0]
		for _, modelName := range intersection {
			if _, ok := available[modelName]; ok {
				filtered = append(filtered, modelName)
			}
		}
		intersection = filtered
	}
	if len(intersection) == 0 {
		return nil, errors.New("所选渠道没有共同支持的模型")
	}
	return intersection, nil
}

func validateMonitorGroupModels(primary string, extra, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, modelName := range allowed {
		allowedSet[modelName] = struct{}{}
	}
	configured := append([]string{strings.TrimSpace(primary)}, extra...)
	for _, modelName := range configured {
		if _, ok := allowedSet[strings.TrimSpace(modelName)]; !ok {
			return errors.New("主探测模型和额外探测模型必须由所选渠道共同支持")
		}
	}
	return nil
}

func monitorChannelTypeNames(channels []*model.Channel) []string {
	seen := make(map[string]struct{}, len(channels))
	typeNames := make([]string, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		typeName := constant.GetChannelTypeName(channel.Type)
		if _, ok := seen[typeName]; ok {
			continue
		}
		seen[typeName] = struct{}{}
		typeNames = append(typeNames, typeName)
	}
	return typeNames
}

func uniqueMonitorGroupChannelIds(channelIds []int) []int {
	seen := make(map[int]struct{}, len(channelIds))
	result := make([]int, 0, len(channelIds))
	for _, channelId := range channelIds {
		if channelId <= 0 {
			continue
		}
		if _, exists := seen[channelId]; exists {
			continue
		}
		seen[channelId] = struct{}{}
		result = append(result, channelId)
	}
	return result
}

func normalizeMonitorGroupModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || len(modelName) > 128 {
			continue
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
	}
	return result
}

func parseMonitorGroupId(c *gin.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, errors.New("监控分组 ID 无效")
	}
	return id, nil
}

func parseMonitorGroupStatus(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	status, err := strconv.Atoi(value)
	if err != nil || (status != 0 && status != 1 && status != 2) {
		return 0, errors.New("invalid status")
	}
	return status, nil
}

func parseMonitorGroupBoundedInt(value string, fallback, min, max int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < min || parsed > max {
		return 0, errors.New("invalid integer")
	}
	return parsed, nil
}
