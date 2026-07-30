package perf_metrics_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerfMetricsSettingDefaultsAndRegistration(t *testing.T) {
	setting := GetPerfMetricsSetting()

	assert.True(t, setting.Enabled)
	assert.Equal(t, 5, setting.FlushInterval)
	assert.Equal(t, 30, setting.RetentionDays)
	assert.Equal(t, 300, GetBucketSeconds())

	registered, ok := config.GlobalConfig.Get("perf_metrics_setting").(*PerfMetricsSetting)
	require.True(t, ok)
	assert.Same(t, setting, registered)
}

func TestGetFlushIntervalMinutesHasMinimum(t *testing.T) {
	original := *GetPerfMetricsSetting()
	t.Cleanup(func() {
		*GetPerfMetricsSetting() = original
	})

	require.NoError(t, config.UpdateConfigFromMap(GetPerfMetricsSetting(), map[string]string{
		"flush_interval": "0",
	}))
	assert.Equal(t, 1, GetFlushIntervalMinutes())

	require.NoError(t, config.UpdateConfigFromMap(GetPerfMetricsSetting(), map[string]string{
		"flush_interval": "7",
	}))
	assert.Equal(t, 7, GetFlushIntervalMinutes())
}
