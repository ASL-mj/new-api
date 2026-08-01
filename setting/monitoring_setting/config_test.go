package monitoring_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoringSettingDefaultsAndRegistration(t *testing.T) {
	setting := GetMonitoringSetting()
	assert.True(t, setting.Enabled)
	assert.Equal(t, 3, setting.ProbeWorkerCount)
	assert.Equal(t, 30, setting.ProbeRetentionDays)
	assert.True(t, setting.OpsEnabled)
	assert.Equal(t, 60, setting.OpsFlushIntervalSeconds)
	assert.True(t, setting.SystemLogEnabled)
	assert.Equal(t, 1, setting.SystemLogInfoSampleRate)
	assert.Equal(t, 14, setting.SystemLogRetentionDays)

	registered, ok := config.GlobalConfig.Get("monitoring_setting").(*MonitoringSetting)
	require.True(t, ok)
	assert.Same(t, setting, registered)
}

func TestMonitoringSettingRuntimeBounds(t *testing.T) {
	original := *GetMonitoringSetting()
	t.Cleanup(func() {
		*GetMonitoringSetting() = original
	})

	require.NoError(t, config.UpdateConfigFromMap(GetMonitoringSetting(), map[string]string{
		"probe_worker_count":          "99",
		"probe_retention_days":        "0",
		"ops_flush_interval_seconds":  "2",
		"ops_retention_days":          "999",
		"system_log_info_sample_rate": "-1",
		"system_log_retention_days":   "0",
	}))
	assert.Equal(t, 8, GetProbeWorkerCount())
	assert.Equal(t, 30, GetProbeRetentionDays())
	assert.Equal(t, 30, GetOpsFlushIntervalSeconds())
	assert.Equal(t, 365, GetOpsRetentionDays())
	assert.Equal(t, 0, GetSystemLogInfoSampleRate())
	assert.Equal(t, 14, GetSystemLogRetentionDays())
}
