package perf_metrics_setting

import "github.com/QuantumNous/new-api/setting/config"

const bucketSeconds = 300

type PerfMetricsSetting struct {
	Enabled       bool `json:"enabled"`
	FlushInterval int  `json:"flush_interval"`
	RetentionDays int  `json:"retention_days"`
}

var perfMetricsSetting = PerfMetricsSetting{
	Enabled:       true,
	FlushInterval: 5,
	RetentionDays: 30,
}

func init() {
	config.GlobalConfig.Register("perf_metrics_setting", &perfMetricsSetting)
}

func GetPerfMetricsSetting() *PerfMetricsSetting {
	return &perfMetricsSetting
}

func GetBucketSeconds() int {
	return bucketSeconds
}

func GetFlushIntervalMinutes() int {
	if perfMetricsSetting.FlushInterval < 1 {
		return 1
	}
	return perfMetricsSetting.FlushInterval
}
