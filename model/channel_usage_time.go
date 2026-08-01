package model

import "time"

// DataExportDefaultTime is a display/export granularity option, not a timezone.
// Daily channel usage follows the process-local timezone until the project adds
// a dedicated dashboard timezone setting.
func channelUsageDateFromTime(at time.Time) string {
	if at.IsZero() {
		at = time.Now()
	}
	loc := time.Local
	if loc == nil {
		loc = time.UTC
	}
	return at.In(loc).Format("2006-01-02")
}
