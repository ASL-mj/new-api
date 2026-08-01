package common

import (
	"sort"
	"sync"
	"time"
)

type JobHeartbeat struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	UpdatedAt int64  `json:"updated_at"`
}

var jobHeartbeats = struct {
	sync.RWMutex
	items map[string]JobHeartbeat
}{items: make(map[string]JobHeartbeat)}

func MarkJobHeartbeat(name string, status string, message string) {
	if name == "" {
		return
	}
	jobHeartbeats.Lock()
	jobHeartbeats.items[name] = JobHeartbeat{Name: name, Status: status, Message: message, UpdatedAt: time.Now().Unix()}
	jobHeartbeats.Unlock()
}

func GetJobHeartbeats() []JobHeartbeat {
	jobHeartbeats.RLock()
	items := make([]JobHeartbeat, 0, len(jobHeartbeats.items))
	for _, heartbeat := range jobHeartbeats.items {
		items = append(items, heartbeat)
	}
	jobHeartbeats.RUnlock()
	sort.Slice(items, func(left, right int) bool { return items[left].Name < items[right].Name })
	return items
}

func IsJobHeartbeatStale(name string, maxAge time.Duration) bool {
	jobHeartbeats.RLock()
	heartbeat, exists := jobHeartbeats.items[name]
	jobHeartbeats.RUnlock()
	return !exists || heartbeat.UpdatedAt == 0 || time.Since(time.Unix(heartbeat.UpdatedAt, 0)) > maxAge
}
