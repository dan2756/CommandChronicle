package dur

import (
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0ms"},
		{850 * time.Millisecond, "850ms"},
		{999*time.Millisecond + 600*time.Microsecond, "1.0s"},
		{1200 * time.Millisecond, "1.2s"},
		{1249 * time.Millisecond, "1.2s"},
		{42 * time.Second, "42.0s"},
		{59960 * time.Millisecond, "1m00s"},
		{3*time.Minute + 4*time.Second, "3m04s"},
		{59*time.Minute + 59*time.Second + 600*time.Millisecond, "1h00m"},
		{time.Hour + 2*time.Minute, "1h02m"},
		{51 * time.Hour, "2d03h"},
		{-1500 * time.Millisecond, "-1.5s"},
	}
	for _, tt := range tests {
		if got := Format(tt.in); got != tt.want {
			t.Errorf("Format(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
