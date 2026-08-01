package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJobHeartbeatTracksFreshAndStaleJobs(t *testing.T) {
	jobHeartbeats.Lock()
	jobHeartbeats.items = make(map[string]JobHeartbeat)
	jobHeartbeats.Unlock()
	t.Cleanup(func() {
		jobHeartbeats.Lock()
		jobHeartbeats.items = make(map[string]JobHeartbeat)
		jobHeartbeats.Unlock()
	})

	assert.True(t, IsJobHeartbeatStale("missing", time.Minute))
	MarkJobHeartbeat("probe", "ok", "")
	assert.False(t, IsJobHeartbeatStale("probe", time.Minute))
	assert.Equal(t, []JobHeartbeat{{Name: "probe", Status: "ok", Message: ""}}, withoutHeartbeatTimestamp(GetJobHeartbeats()))
}

func withoutHeartbeatTimestamp(items []JobHeartbeat) []JobHeartbeat {
	for index := range items {
		items[index].UpdatedAt = 0
	}
	return items
}
