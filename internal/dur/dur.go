// Package dur parses and formats durations the way cmdtime's flags and
// reports use them.
package dur

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

var units = map[string]time.Duration{
	"ms": time.Millisecond,
	"s":  time.Second,
	"m":  time.Minute,
	"h":  time.Hour,
	"d":  24 * time.Hour,
	"w":  7 * 24 * time.Hour,
}

var (
	bareNumber = regexp.MustCompile(`^\d+(\.\d+)?$`)
	wholeDur   = regexp.MustCompile(`^(\d+(\.\d+)?(ms|s|m|h|d|w))+$`)
	component  = regexp.MustCompile(`(\d+(?:\.\d+)?)(ms|s|m|h|d|w)`)
)

// Parse reads durations like "500ms", "1.5s", "3m", "1h30m", "7d" or "2w".
// A bare number is a number of seconds.
func Parse(s string) (time.Duration, error) {
	if bareNumber.MatchString(s) {
		s += "s"
	}
	if !wholeDur.MatchString(s) {
		return 0, fmt.Errorf("invalid duration %q (try 500ms, 30s, 5m, 2h or 7d)", s)
	}
	var total float64
	for _, m := range component.FindAllStringSubmatch(s, -1) {
		n, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		total += n * float64(units[m[2]])
	}
	if total >= math.MaxInt64 {
		return 0, fmt.Errorf("duration %q is too large", s)
	}
	return time.Duration(total), nil
}

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
