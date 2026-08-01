package model

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var channelUsageTimezoneCache struct {
	sync.RWMutex
	name string
	loc  *time.Location
}

func channelUsageDateFromTime(at time.Time) (string, error) {
	if at.IsZero() {
		at = time.Now()
	}

	loc, err := getChannelUsageLocation()
	if err != nil {
		return "", err
	}
	return at.In(loc).Format("2006-01-02"), nil
}

func getChannelUsageLocation() (*time.Location, error) {
	timezone := getChannelUsageTimezoneName()

	channelUsageTimezoneCache.RLock()
	if channelUsageTimezoneCache.name == timezone && channelUsageTimezoneCache.loc != nil {
		loc := channelUsageTimezoneCache.loc
		channelUsageTimezoneCache.RUnlock()
		return loc, nil
	}
	channelUsageTimezoneCache.RUnlock()

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid channel usage timezone %q: %w", timezone, err)
	}

	channelUsageTimezoneCache.Lock()
	channelUsageTimezoneCache.name = timezone
	channelUsageTimezoneCache.loc = loc
	channelUsageTimezoneCache.Unlock()
	return loc, nil
}

func getChannelUsageTimezoneName() string {
	common.OptionMapRWMutex.RLock()
	if common.OptionMap != nil {
		if timezone := strings.TrimSpace(common.OptionMap["ChannelUsageTimezone"]); timezone != "" {
			common.OptionMapRWMutex.RUnlock()
			return timezone
		}
	}
	common.OptionMapRWMutex.RUnlock()

	if timezone := strings.TrimSpace(os.Getenv("CHANNEL_USAGE_TIMEZONE")); timezone != "" {
		return timezone
	}

	return strings.TrimSpace(common.ChannelUsageTimezone)
}

func resetChannelUsageTimezoneCache() {
	channelUsageTimezoneCache.Lock()
	channelUsageTimezoneCache.name = ""
	channelUsageTimezoneCache.loc = nil
	channelUsageTimezoneCache.Unlock()
}
