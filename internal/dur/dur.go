// Package dur formats durations the way cmdtime prints them.
package dur

import (
	"fmt"
	"time"
)

// Format renders d compactly with at most two units:
// 850ms, 1.2s, 42.0s, 3m04s, 1h02m, 2d03h.
func Format(d time.Duration) string {
	if d < 0 {
		return "-" + Format(-d)
	}
	ms := d.Round(time.Millisecond).Milliseconds()
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if tenths := (ms + 50) / 100; tenths < 600 {
		return fmt.Sprintf("%d.%ds", tenths/10, tenths%10)
	}
	secs := (ms + 500) / 1000
	if secs < 3600 {
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	}
	mins := (secs + 30) / 60
	if mins < 24*60 {
		return fmt.Sprintf("%dh%02dm", mins/60, mins%60)
	}
	hours := (mins + 30) / 60
	return fmt.Sprintf("%dd%02dh", hours/24, hours%24)
}
