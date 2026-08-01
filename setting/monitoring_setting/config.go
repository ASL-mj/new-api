package monitoring_setting

import "github.com/QuantumNous/new-api/setting/config"

type MonitoringSetting struct {
	Enabled                 bool `json:"enabled"`
	ProbeWorkerCount        int  `json:"probe_worker_count"`
	ProbeRetentionDays      int  `json:"probe_retention_days"`
	OpsEnabled              bool `json:"ops_enabled"`
	OpsFlushIntervalSeconds int  `json:"ops_flush_interval_seconds"`
	OpsRetentionDays        int  `json:"ops_retention_days"`
	SystemLogEnabled        bool `json:"system_log_enabled"`
	SystemLogInfoSampleRate int  `json:"system_log_info_sample_rate"`
	SystemLogRetentionDays  int  `json:"system_log_retention_days"`
}

var monitoringSetting = MonitoringSetting{
	Enabled:                 true,
	ProbeWorkerCount:        3,
	ProbeRetentionDays:      30,
	OpsEnabled:              true,
	OpsFlushIntervalSeconds: 60,
	OpsRetentionDays:        30,
	SystemLogEnabled:        true,
	SystemLogInfoSampleRate: 1,
	SystemLogRetentionDays:  14,
}

func init() {
	config.GlobalConfig.Register("monitoring_setting", &monitoringSetting)
}

func GetMonitoringSetting() *MonitoringSetting {
	return &monitoringSetting
}

func GetProbeWorkerCount() int {
	count := monitoringSetting.ProbeWorkerCount
	if count < 1 {
		return 1
	}
	if count > 8 {
		return 8
	}
	return count
}

func GetProbeRetentionDays() int {
	return normalizeRetentionDays(monitoringSetting.ProbeRetentionDays, 30)
}

func GetOpsFlushIntervalSeconds() int {
	seconds := monitoringSetting.OpsFlushIntervalSeconds
	if seconds < 30 {
		return 30
	}
	if seconds > 300 {
		return 300
	}
	return seconds
}

func GetOpsRetentionDays() int {
	return normalizeRetentionDays(monitoringSetting.OpsRetentionDays, 30)
}

func GetSystemLogInfoSampleRate() int {
	rate := monitoringSetting.SystemLogInfoSampleRate
	if rate < 0 {
		return 0
	}
	if rate > 100 {
		return 100
	}
	return rate
}

func GetSystemLogRetentionDays() int {
	return normalizeRetentionDays(monitoringSetting.SystemLogRetentionDays, 14)
}

func normalizeRetentionDays(days int, fallback int) int {
	if days < 1 {
		return fallback
	}
	if days > 365 {
		return 365
	}
	return days
}
