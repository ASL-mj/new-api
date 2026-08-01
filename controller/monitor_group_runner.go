package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/monitoring_setting"
)

const monitorGroupSchedulerInterval = 5 * time.Second

var errMonitorGroupRunning = errors.New("monitor group is already running")

type monitorGroupRunnerState struct {
	ctx              context.Context
	jobs             chan monitorProbeJob
	workerNum        int
	schedulerStarted bool
	wg               sync.WaitGroup
}

type monitorProbeJob struct {
	group     *model.MonitorGroup
	channel   *model.Channel
	modelName string
	pingMs    *int64
	resultCh  chan<- model.MonitorCheck
}

var monitorGroupRunner = struct {
	sync.Mutex
	state  *monitorGroupRunnerState
	cancel context.CancelFunc
}{}

var runningMonitorGroups sync.Map

// monitorProbeFunc is replaceable by focused tests so scheduling is tested without an upstream call.
var monitorProbeFunc = probeChannel
var monitorEndpointPingFunc = pingMonitorEndpointOrigin
var recordMonitorSystemEvent = service.RecordSystemEvent

func isMonitorGroupRunning(groupId int) bool {
	_, running := runningMonitorGroups.Load(groupId)
	return running
}

func StartMonitorGroupRunner() {
	monitorGroupRunner.Lock()
	state := monitorGroupRunner.state
	startWorkers := false
	if state == nil {
		ctx, cancel := context.WithCancel(context.Background())
		state = &monitorGroupRunnerState{
			ctx:       ctx,
			jobs:      make(chan monitorProbeJob, 128),
			workerNum: monitoring_setting.GetProbeWorkerCount(),
		}
		monitorGroupRunner.state = state
		monitorGroupRunner.cancel = cancel
		startWorkers = true
	}
	startScheduler := common.IsMasterNode && !state.schedulerStarted
	if startScheduler {
		state.schedulerStarted = true
	}
	monitorGroupRunner.Unlock()

	if startWorkers {
		common.SysLog(fmt.Sprintf("monitor group workers started: %d", state.workerNum))
		for i := 0; i < state.workerNum; i++ {
			state.wg.Add(1)
			go func() {
				defer state.wg.Done()
				monitorProbeWorker(state)
			}()
		}
	}
	if startScheduler {
		common.SysLog("monitor group scheduler started on master node")
		state.wg.Add(1)
		go func() {
			defer state.wg.Done()
			monitorGroupScheduler(state)
		}()
	} else if startWorkers && !common.IsMasterNode {
		common.SysLog("monitor group scheduler skipped on slave node")
	}
}

func StopMonitorGroupRunnerForTest() {
	monitorGroupRunner.Lock()
	state := monitorGroupRunner.state
	cancel := monitorGroupRunner.cancel
	monitorGroupRunner.state = nil
	monitorGroupRunner.cancel = nil
	monitorGroupRunner.Unlock()
	if cancel != nil {
		cancel()
	}
	if state != nil {
		state.wg.Wait()
	}
	runningMonitorGroups.Range(func(key, _ interface{}) bool {
		runningMonitorGroups.Delete(key)
		return true
	})
}

func RunMonitorGroupNow(groupId int) error {
	if groupId <= 0 {
		return errors.New("invalid monitor group id")
	}
	group, err := model.GetMonitorGroupById(groupId)
	if err != nil {
		return err
	}
	return scheduleMonitorGroupRun(group)
}

func monitorGroupScheduler(state *monitorGroupRunnerState) {
	runDueMonitorGroups(state)
	ticker := time.NewTicker(monitorGroupSchedulerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-state.ctx.Done():
			return
		case <-ticker.C:
			runDueMonitorGroups(state)
		}
	}
}

func runDueMonitorGroups(state *monitorGroupRunnerState) {
	common.MarkJobHeartbeat("monitor_group_runner", "ok", "")
	if !monitoring_setting.GetMonitoringSetting().Enabled {
		return
	}
	groups, err := model.GetEnabledMonitorGroups()
	if err != nil {
		common.SysError("failed to load enabled monitor groups: " + err.Error())
		return
	}
	now := time.Now().Unix()
	for _, group := range groups {
		if group.LastCheckedAt > 0 && group.LastCheckedAt+int64(group.IntervalSeconds) > now {
			continue
		}
		if err := scheduleMonitorGroupRunWithState(state, group); err != nil && !errors.Is(err, errMonitorGroupRunning) {
			common.SysError(fmt.Sprintf("failed to schedule monitor group %d: %v", group.Id, err))
		}
	}
}

func scheduleMonitorGroupRun(group *model.MonitorGroup) error {
	monitorGroupRunner.Lock()
	state := monitorGroupRunner.state
	monitorGroupRunner.Unlock()
	if state == nil {
		return errors.New("monitor group runner is not started")
	}
	return scheduleMonitorGroupRunWithState(state, group)
}

func scheduleMonitorGroupRunWithState(state *monitorGroupRunnerState, group *model.MonitorGroup) error {
	if group == nil || group.Id <= 0 {
		return errors.New("invalid monitor group")
	}
	if _, loaded := runningMonitorGroups.LoadOrStore(group.Id, struct{}{}); loaded {
		return errMonitorGroupRunning
	}
	go func() {
		defer runningMonitorGroups.Delete(group.Id)
		runMonitorGroup(state, group)
	}()
	return nil
}

func runMonitorGroup(state *monitorGroupRunnerState, group *model.MonitorGroup) {
	recordMonitorGroupEvent(group, "info", "监控分组开始探测")
	targets, err := model.GetMonitorGroupTargets(group.Id)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to load monitor targets for group %d: %v", group.Id, err))
		recordMonitorGroupEvent(group, "error", "监控分组探测失败：无法读取渠道配置")
		return
	}

	checks := make([]*model.MonitorCheck, 0)
	jobs := make([]monitorProbeJob, 0)
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		models := getMonitorTargetModels(group, target)
		if len(models) == 0 {
			continue
		}
		channel, channelErr := model.GetChannelById(target.ChannelId, true)
		if channelErr != nil {
			for _, modelName := range models {
				checks = append(checks, &model.MonitorCheck{
					MonitorGroupId: group.Id,
					ChannelId:      target.ChannelId,
					ModelName:      modelName,
					Status:         model.MonitorCheckStatusFailed,
					ErrorCode:      "channel_not_found",
					ErrorMessage:   "configured channel is no longer available",
					CheckedAt:      time.Now().Unix(),
				})
			}
			continue
		}
		pingTimeout := time.Duration(group.TimeoutSeconds) * time.Second
		if pingTimeout <= 0 || pingTimeout > 5*time.Second {
			pingTimeout = 5 * time.Second
		}
		pingMs := monitorEndpointPingFunc(channel, pingTimeout)
		for _, modelName := range models {
			jobs = append(jobs, monitorProbeJob{group: group, channel: channel, modelName: modelName, pingMs: pingMs})
		}
	}

	resultCh := make(chan model.MonitorCheck, len(jobs))
	pending := 0
	for _, job := range jobs {
		job.resultCh = resultCh
		select {
		case <-state.ctx.Done():
			return
		case state.jobs <- job:
			pending++
		}
	}

	for i := 0; i < pending; i++ {
		select {
		case <-state.ctx.Done():
			return
		case check := <-resultCh:
			checks = append(checks, &check)
		}
	}

	if len(checks) > 0 {
		if err := model.InsertMonitorChecks(checks); err != nil {
			common.SysError(fmt.Sprintf("failed to save monitor checks for group %d: %v", group.Id, err))
			recordMonitorGroupEvent(group, "error", "监控分组探测失败：无法保存探测结果")
			return
		}
	}
	if err := model.DB.Model(&model.MonitorGroup{}).Where("id = ?", group.Id).Update("last_checked_at", time.Now().Unix()).Error; err != nil {
		common.SysError(fmt.Sprintf("failed to update monitor group %d check time: %v", group.Id, err))
		recordMonitorGroupEvent(group, "error", "监控分组探测失败：无法更新完成时间")
		return
	}
	recordMonitorGroupEvent(group, monitorGroupCompletionLevel(checks), monitorGroupCompletionMessage(checks))
}

func recordMonitorGroupEvent(group *model.MonitorGroup, level, message string) {
	if group == nil {
		return
	}
	recordMonitorSystemEvent(model.SystemEventLog{
		CreatedAt: common.GetTimestamp(), Level: level, Component: "monitor_group",
		Message:   fmt.Sprintf("监控分组 #%d（%s）：%s", group.Id, group.Name, message),
		ModelName: group.PrimaryModel,
	})
}

func monitorGroupCompletionLevel(checks []*model.MonitorCheck) string {
	for _, check := range checks {
		if check.Status == model.MonitorCheckStatusFailed || check.Status == model.MonitorCheckStatusTimeout {
			return "warn"
		}
	}
	return "info"
}

func monitorGroupCompletionMessage(checks []*model.MonitorCheck) string {
	if len(checks) == 0 {
		return "探测完成，没有可执行的渠道或模型"
	}
	operational, degraded, failed := 0, 0, 0
	for _, check := range checks {
		switch check.Status {
		case model.MonitorCheckStatusOperational:
			operational++
		case model.MonitorCheckStatusDegraded:
			degraded++
		default:
			failed++
		}
	}
	return fmt.Sprintf("探测完成：正常 %d，降级 %d，失败或超时 %d", operational, degraded, failed)
}

func monitorProbeWorker(state *monitorGroupRunnerState) {
	for {
		select {
		case <-state.ctx.Done():
			return
		case job := <-state.jobs:
			check := runMonitorProbeSafely(job)
			select {
			case <-state.ctx.Done():
				return
			case job.resultCh <- check:
			}
		}
	}
}

func runMonitorProbeSafely(job monitorProbeJob) (check model.MonitorCheck) {
	check = model.MonitorCheck{
		MonitorGroupId: job.group.Id,
		ChannelId:      job.channel.Id,
		ModelName:      job.modelName,
		PingLatencyMs:  job.pingMs,
		CheckedAt:      time.Now().Unix(),
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			check.Status = model.MonitorCheckStatusFailed
			check.ErrorCode = "probe_panic"
			check.ErrorMessage = fmt.Sprintf("probe panic: %v", recovered)
		}
	}()

	result := monitorProbeFunc(job.channel, job.modelName, time.Duration(job.group.TimeoutSeconds)*time.Second)
	check.LatencyMs = result.LatencyMs
	if result.Success {
		degradedThreshold := job.group.DegradedMs
		if degradedThreshold <= 0 {
			degradedThreshold = 3000
		}
		if result.LatencyMs >= int64(degradedThreshold) {
			check.Status = model.MonitorCheckStatusDegraded
		} else {
			check.Status = model.MonitorCheckStatusOperational
		}
		return check
	}

	check.ErrorCode = result.ErrorCode
	check.ErrorMessage = common.MaskSensitiveInfo(result.Message)
	if result.ErrorCode == "timeout" {
		check.Status = model.MonitorCheckStatusTimeout
	} else {
		check.Status = model.MonitorCheckStatusFailed
	}
	return check
}

func pingMonitorEndpointOrigin(channel *model.Channel, timeout time.Duration) *int64 {
	if channel == nil {
		return nil
	}
	parsed, err := url.Parse(strings.TrimSpace(channel.GetBaseURL()))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}
	origin := parsed.Scheme + "://" + parsed.Host
	request, err := http.NewRequestWithContext(context.Background(), http.MethodHead, origin, nil)
	if err != nil {
		return nil
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	startedAt := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return nil
	}
	_ = response.Body.Close()
	latency := time.Since(startedAt).Milliseconds()
	return &latency
}

func getMonitorTargetModels(group *model.MonitorGroup, target *model.MonitorGroupTarget) []string {
	configured := ""
	if target != nil {
		configured = target.Models
	}
	if strings.TrimSpace(configured) == "" && group != nil {
		configured = group.ExtraModels
	}

	models := make([]string, 0)
	if target == nil || strings.TrimSpace(target.Models) == "" {
		if group != nil && strings.TrimSpace(group.PrimaryModel) != "" {
			models = append(models, strings.TrimSpace(group.PrimaryModel))
		}
	}
	if strings.TrimSpace(configured) != "" {
		var extraModels []string
		if err := common.Unmarshal([]byte(configured), &extraModels); err != nil {
			common.SysError(fmt.Sprintf("invalid monitor models JSON for group %d: %v", group.Id, err))
			return models
		}
		models = append(models, extraModels...)
	}

	seen := make(map[string]struct{}, len(models))
	filtered := make([]string, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		filtered = append(filtered, modelName)
	}
	return filtered
}
