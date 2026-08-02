package controller

import (
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	opsmetrics "github.com/QuantumNous/new-api/pkg/ops_metrics"
	"github.com/gin-gonic/gin"
)

const userMonitorTimelineLimit = 60

type UserMonitorTimeline struct {
	Status    string `json:"status"`
	CheckedAt int64  `json:"checked_at"`
}

type UserMonitorGroupSummary struct {
	Id                 int                   `json:"id"`
	Name               string                `json:"name"`
	Status             string                `json:"status"`
	PrimaryModel       string                `json:"primary_model"`
	ChannelTypes       []string              `json:"channel_types"`
	CurrentLatencyMs   int64                 `json:"current_latency_ms"`
	CurrentPingLatency *int64                `json:"current_ping_latency_ms"`
	RealSuccessRate    *float64              `json:"real_success_rate"`
	Availability7d     float64               `json:"availability_7d"`
	Availability15d    float64               `json:"availability_15d"`
	Availability30d    float64               `json:"availability_30d"`
	AvailabilityDays   int                   `json:"availability_days"`
	Availability       float64               `json:"availability"`
	Timeline           []UserMonitorTimeline `json:"timeline"`
}

type UserMonitorGroupDetail struct {
	UserMonitorGroupSummary
	AvailabilityHistory []model.MonitorAvailability `json:"availability_history"`
}

func GetMonitorStatus(c *gin.Context) {
	days, err := parseUserMonitorStatusDays(c.Query("days"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groups, err := getUserVisibleMonitorGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	summaries, err := buildUserMonitorGroupSummaries(groups, days)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summaries)
}

func GetMonitorStatusGroup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgMonitorGroupIdInvalid)
		return
	}
	days, err := parseUserMonitorStatusDays(c.Query("days"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groups, err := getUserVisibleMonitorGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, group := range groups {
		if group.Id != id {
			continue
		}
		summaries, err := buildUserMonitorGroupSummaries([]*model.MonitorGroup{group}, days)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		availability, err := model.GetMonitorAvailability(group.Id, days)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, UserMonitorGroupDetail{
			UserMonitorGroupSummary: summaries[0], AvailabilityHistory: availability,
		})
		return
	}
	common.ApiErrorI18n(c, i18n.MsgMonitorStatusNotFound)
}

func getUserVisibleMonitorGroups() ([]*model.MonitorGroup, error) {
	groups := make([]*model.MonitorGroup, 0)
	err := model.DB.Where("enabled = ? AND user_visible = ?", true, true).Order("id ASC").Find(&groups).Error
	return groups, err
}

func buildUserMonitorGroupSummaries(groups []*model.MonitorGroup, days int) ([]UserMonitorGroupSummary, error) {
	summaries := make([]UserMonitorGroupSummary, len(groups))
	if len(groups) == 0 {
		return summaries, nil
	}

	groupIds := make([]int, 0, len(groups))
	for _, group := range groups {
		groupIds = append(groupIds, group.Id)
	}
	targets, err := model.GetMonitorGroupTargetsByGroupIds(groupIds)
	if err != nil {
		return nil, err
	}
	checks, err := model.GetMonitorChecksForStatus(groupIds, time.Now().Add(-30*24*time.Hour).Unix())
	if err != nil {
		return nil, err
	}

	channelIDsByGroup := make(map[int][]int, len(groups))
	for _, target := range targets {
		if target.Enabled {
			channelIDsByGroup[target.MonitorGroupId] = append(channelIDsByGroup[target.MonitorGroupId], target.ChannelId)
		}
	}
	allChannelIds := make([]int, 0)
	for _, channelIds := range channelIDsByGroup {
		allChannelIds = append(allChannelIds, channelIds...)
	}
	channels, err := model.GetChannelsByIds(uniqueMonitorGroupChannelIds(allChannelIds))
	if err != nil {
		return nil, err
	}
	channelById := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		channelById[channel.Id] = channel
	}
	checksByGroup := make(map[int][]*model.MonitorCheck, len(groups))
	for _, check := range checks {
		checksByGroup[check.MonitorGroupId] = append(checksByGroup[check.MonitorGroupId], check)
	}
	successRateQueries := make(map[int]opsmetrics.ChannelSuccessRateQuery, len(groups))
	for _, group := range groups {
		successRateQueries[group.Id] = opsmetrics.ChannelSuccessRateQuery{
			ChannelIDs: channelIDsByGroup[group.Id],
			Model:      group.PrimaryModel,
		}
	}
	realSuccessRates, err := opsmetrics.QueryChannelSuccessRates(successRateQueries, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	for index, group := range groups {
		groupChecks := checksByGroup[group.Id]
		freshChecks := currentMonitorChecks(groupChecks, group, now)
		availability7d := monitorAvailabilityForDays(groupChecks, now, 7)
		availability15d := monitorAvailabilityForDays(groupChecks, now, 15)
		availability30d := monitorAvailabilityForDays(groupChecks, now, 30)
		availability := availability30d
		switch days {
		case 7:
			availability = availability7d
		case 15:
			availability = availability15d
		}
		summaries[index] = UserMonitorGroupSummary{
			Id:                 group.Id,
			Name:               group.Name,
			Status:             summarizeMonitorStatus(freshChecks),
			PrimaryModel:       group.PrimaryModel,
			ChannelTypes:       monitorTypesForChannelIds(channelIDsByGroup[group.Id], channelById),
			CurrentLatencyMs:   averageCurrentModelLatency(freshChecks, group.PrimaryModel),
			CurrentPingLatency: averageCurrentPingLatency(freshChecks, group.PrimaryModel),
			RealSuccessRate:    realSuccessRates[group.Id],
			Availability7d:     availability7d,
			Availability15d:    availability15d,
			Availability30d:    availability30d,
			AvailabilityDays:   days,
			Availability:       availability,
			Timeline:           summarizeUserMonitorTimeline(groupChecks),
		}
	}
	return summaries, nil
}

func currentMonitorChecks(checks []*model.MonitorCheck, group *model.MonitorGroup, now int64) []*model.MonitorCheck {
	if group == nil {
		return []*model.MonitorCheck{}
	}
	freshFor := int64(group.IntervalSeconds + group.TimeoutSeconds)
	if freshFor < 15 {
		freshFor = 15
	}
	cutoff := now - freshFor
	fresh := make([]*model.MonitorCheck, 0, len(checks))
	for _, check := range checks {
		if check != nil && check.CheckedAt >= cutoff {
			fresh = append(fresh, check)
		}
	}
	return fresh
}

func parseUserMonitorStatusDays(value string) (int, error) {
	if value == "" {
		return 30, nil
	}
	days, err := strconv.Atoi(value)
	if err != nil || (days != 7 && days != 15 && days != 30) {
		return 0, common.NewLocalizedError(i18n.MsgMonitorStatusDaysInvalid)
	}
	return days, nil
}

func summarizeMonitorStatus(checks []*model.MonitorCheck) string {
	if len(checks) == 0 {
		return "unknown"
	}
	latestByTarget := make(map[string]*model.MonitorCheck)
	for _, check := range checks {
		key := strconv.Itoa(check.ChannelId) + "\x00" + check.ModelName
		latestByTarget[key] = check
	}
	available := 0
	degraded := false
	allTimeout := true
	for _, check := range latestByTarget {
		switch check.Status {
		case model.MonitorCheckStatusOperational:
			available++
		case model.MonitorCheckStatusDegraded:
			available++
			degraded = true
		case model.MonitorCheckStatusTimeout:
			degraded = true
		default:
			degraded = true
			allTimeout = false
		}
		if check.Status != model.MonitorCheckStatusTimeout {
			allTimeout = false
		}
	}
	if available == 0 {
		if allTimeout {
			return model.MonitorCheckStatusTimeout
		}
		return model.MonitorCheckStatusFailed
	}
	if degraded {
		return model.MonitorCheckStatusDegraded
	}
	return model.MonitorCheckStatusOperational
}

func averageCurrentModelLatency(checks []*model.MonitorCheck, modelName string) int64 {
	latestByChannel := make(map[int]*model.MonitorCheck)
	for _, check := range checks {
		if check.ModelName != modelName || (check.Status != model.MonitorCheckStatusOperational && check.Status != model.MonitorCheckStatusDegraded) {
			continue
		}
		latestByChannel[check.ChannelId] = check
	}
	var total int64
	var count int64
	for _, check := range latestByChannel {
		total += check.LatencyMs
		count++
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func averageCurrentPingLatency(checks []*model.MonitorCheck, modelName string) *int64 {
	latestByChannel := make(map[int]*model.MonitorCheck)
	for _, check := range checks {
		if check.ModelName != modelName || check.PingLatencyMs == nil || (check.Status != model.MonitorCheckStatusOperational && check.Status != model.MonitorCheckStatusDegraded) {
			continue
		}
		latestByChannel[check.ChannelId] = check
	}
	if len(latestByChannel) == 0 {
		return nil
	}
	var total int64
	for _, check := range latestByChannel {
		total += *check.PingLatencyMs
	}
	average := total / int64(len(latestByChannel))
	return &average
}

func monitorTypesForChannelIds(channelIds []int, channelById map[int]*model.Channel) []string {
	channels := make([]*model.Channel, 0, len(channelIds))
	for _, channelId := range channelIds {
		if channel := channelById[channelId]; channel != nil {
			channels = append(channels, channel)
		}
	}
	return monitorChannelTypeNames(channels)
}

func monitorAvailabilityForDays(checks []*model.MonitorCheck, now int64, days int) float64 {
	if days <= 0 {
		return 0
	}
	cutoff := now - int64(days)*24*60*60
	var total int64
	var available int64
	for _, check := range checks {
		if check.CheckedAt < cutoff {
			continue
		}
		total++
		if check.Status == model.MonitorCheckStatusOperational || check.Status == model.MonitorCheckStatusDegraded {
			available++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(available) * 100 / float64(total)
}

func summarizeUserMonitorTimeline(checks []*model.MonitorCheck) []UserMonitorTimeline {
	byTimestamp := make(map[int64]string)
	for _, check := range checks {
		current, exists := byTimestamp[check.CheckedAt]
		if !exists || monitorStatusPriority(check.Status) > monitorStatusPriority(current) {
			byTimestamp[check.CheckedAt] = check.Status
		}
	}
	timestamps := make([]int64, 0, len(byTimestamp))
	for timestamp := range byTimestamp {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	if len(timestamps) > userMonitorTimelineLimit {
		timestamps = timestamps[len(timestamps)-userMonitorTimelineLimit:]
	}
	timeline := make([]UserMonitorTimeline, 0, len(timestamps))
	for _, timestamp := range timestamps {
		timeline = append(timeline, UserMonitorTimeline{Status: byTimestamp[timestamp], CheckedAt: timestamp})
	}
	return timeline
}

func monitorStatusPriority(status string) int {
	switch status {
	case model.MonitorCheckStatusFailed:
		return 4
	case model.MonitorCheckStatusTimeout:
		return 3
	case model.MonitorCheckStatusDegraded:
		return 2
	case model.MonitorCheckStatusOperational:
		return 1
	default:
		return 0
	}
}
