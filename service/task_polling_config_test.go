package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadTaskPollingV2ConfigDefaultsToClosedLegacySafeValues(t *testing.T) {
	t.Setenv("TASK_POLLING_V2_ENABLED", "")
	t.Setenv("TASK_POLL_MAX_FAILURES", "")
	t.Setenv("TASK_POLL_BASE_BACKOFF_SECONDS", "")
	t.Setenv("TASK_POLL_MAX_BACKOFF_SECONDS", "")

	config := loadTaskPollingV2Config()
	assert.False(t, config.Enabled)
	assert.Equal(t, 20, config.MaxFailures)
	assert.Equal(t, 10, config.BaseBackoffSeconds)
	assert.Equal(t, 300, config.MaxBackoffSeconds)
}

func TestLoadTaskPollingV2ConfigRejectsInvalidNumericOverrides(t *testing.T) {
	t.Setenv("TASK_POLLING_V2_ENABLED", "true")
	t.Setenv("TASK_POLL_MAX_FAILURES", "0")
	t.Setenv("TASK_POLL_BASE_BACKOFF_SECONDS", "-5")
	t.Setenv("TASK_POLL_MAX_BACKOFF_SECONDS", "3")

	config := loadTaskPollingV2Config()
	assert.True(t, config.Enabled)
	assert.Equal(t, 20, config.MaxFailures)
	assert.Equal(t, 10, config.BaseBackoffSeconds)
	assert.Equal(t, 300, config.MaxBackoffSeconds)
}

func TestLoadTaskPollingV2ConfigAcceptsValidOverrides(t *testing.T) {
	t.Setenv("TASK_POLLING_V2_ENABLED", "true")
	t.Setenv("TASK_POLL_MAX_FAILURES", "8")
	t.Setenv("TASK_POLL_BASE_BACKOFF_SECONDS", "15")
	t.Setenv("TASK_POLL_MAX_BACKOFF_SECONDS", "120")

	config := loadTaskPollingV2Config()
	assert.True(t, config.Enabled)
	assert.Equal(t, 8, config.MaxFailures)
	assert.Equal(t, 15, config.BaseBackoffSeconds)
	assert.Equal(t, 120, config.MaxBackoffSeconds)
}

func TestTaskPollingV2ChannelGate(t *testing.T) {
	t.Setenv("TASK_POLLING_V2_ENABLED", "true")
	t.Setenv("TASK_POLLING_V2_CHANNEL_IDS", "51, 52")
	assert.True(t, TaskPollingV2EnabledForChannel(51))
	assert.True(t, TaskPollingV2EnabledForChannel(52))
	assert.False(t, TaskPollingV2EnabledForChannel(53))
	t.Setenv("TASK_POLLING_V2_CHANNEL_IDS", "invalid")
	assert.False(t, TaskPollingV2EnabledForChannel(51))
	t.Setenv("TASK_POLLING_V2_CHANNEL_IDS", "")
	assert.True(t, TaskPollingV2EnabledForChannel(51))
	t.Setenv("TASK_POLLING_V2_ENABLED", "false")
	assert.False(t, TaskPollingV2EnabledForChannel(51))
}

func TestTaskPollingV2BackoffMaximumAtLeastBase(t *testing.T) {
	t.Setenv("TASK_POLL_BASE_BACKOFF_SECONDS", "400")
	t.Setenv("TASK_POLL_MAX_BACKOFF_SECONDS", "3")
	config := loadTaskPollingV2Config()
	assert.GreaterOrEqual(t, config.MaxBackoffSeconds, config.BaseBackoffSeconds)
}
