package service

import (
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	defaultTaskPollMaxFailures        = 20
	defaultTaskPollBaseBackoffSeconds = 10
	defaultTaskPollMaxBackoffSeconds  = 300
)

type taskPollingV2Config struct {
	Enabled            bool
	MaxFailures        int
	BaseBackoffSeconds int
	MaxBackoffSeconds  int
	ChannelIDs         map[int]bool
}

// Enabling V2 selects new tasks only. Persisted V2 tasks keep their polling
// version after a rollout switch is turned off, until they reach a terminal state.
func loadTaskPollingV2Config() taskPollingV2Config {
	config := taskPollingV2Config{
		Enabled:            common.GetEnvOrDefaultBool("TASK_POLLING_V2_ENABLED", false),
		MaxFailures:        common.GetEnvOrDefault("TASK_POLL_MAX_FAILURES", defaultTaskPollMaxFailures),
		BaseBackoffSeconds: common.GetEnvOrDefault("TASK_POLL_BASE_BACKOFF_SECONDS", defaultTaskPollBaseBackoffSeconds),
		MaxBackoffSeconds:  common.GetEnvOrDefault("TASK_POLL_MAX_BACKOFF_SECONDS", defaultTaskPollMaxBackoffSeconds),
	}
	if config.MaxFailures <= 0 {
		config.MaxFailures = defaultTaskPollMaxFailures
	}
	if config.BaseBackoffSeconds <= 0 {
		config.BaseBackoffSeconds = defaultTaskPollBaseBackoffSeconds
	}
	if config.MaxBackoffSeconds < config.BaseBackoffSeconds {
		config.MaxBackoffSeconds = defaultTaskPollMaxBackoffSeconds
		if config.MaxBackoffSeconds < config.BaseBackoffSeconds {
			config.MaxBackoffSeconds = config.BaseBackoffSeconds
		}
	}
	if raw := strings.TrimSpace(os.Getenv("TASK_POLLING_V2_CHANNEL_IDS")); raw != "" {
		config.ChannelIDs = make(map[int]bool)
		for _, value := range strings.Split(raw, ",") {
			if id, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && id > 0 {
				config.ChannelIDs[id] = true
			}
		}
	}
	return config
}

func TaskPollingV2EnabledForChannel(channelID int) bool {
	config := loadTaskPollingV2Config()
	return config.Enabled && (config.ChannelIDs == nil || config.ChannelIDs[channelID])
}
