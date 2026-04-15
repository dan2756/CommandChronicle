package stats

import (
	"testing"
	"time"
)

func secs(vals ...float64) []time.Duration {
	out := make([]time.Duration, len(vals))
	for i, v := range vals {
		out[i] = time.Duration(v * float64(time.Second))
	}
	return out
}

func TestPercentile(t *testing.T) {
	oneToTen := secs(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	tests := []struct {
		name   string
		sorted []time.Duration
		p      float64
		want   time.Duration
	}{
		{"empty", nil, 50, 0},
		{"single", secs(7), 95, 7 * time.Second},
		{"min", oneToTen, 0, time.Second},
		{"max", oneToTen, 100, 10 * time.Second},
		{"median even", oneToTen, 50, 5500 * time.Millisecond},
		{"median odd", secs(1, 2, 3), 50, 2 * time.Second},
		{"p95", oneToTen, 95, 9550 * time.Millisecond},
		{"p90", oneToTen, 90, 9100 * time.Millisecond},
		{"p25 of four", secs(0.1, 0.2, 0.3, 0.4), 25, 175 * time.Millisecond},
		{"ties", secs(2, 2, 2, 9), 50, 2 * time.Second},
	}
	for _, tt := range tests {
		got := Percentile(tt.sorted, tt.p)
		if diff := got - tt.want; diff < -time.Microsecond || diff > time.Microsecond {
			t.Errorf("%s: Percentile(p%v) = %v, want %v", tt.name, tt.p, got, tt.want)
		}
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize(secs(4, 1, 3, 2), 1) // deliberately unsorted
	want := Summary{
		Count:    4,
		Failures: 1,
		Total:    10 * time.Second,
		Mean:     2500 * time.Millisecond,
		P50:      2500 * time.Millisecond,
		P95:      3850 * time.Millisecond,
		Max:      4 * time.Second,
	}
	if s != want {
		t.Errorf("Summarize = %+v\nwant        %+v", s, want)
	}
	if got := s.FailureRate(); got != 0.25 {
		t.Errorf("FailureRate = %v, want 0.25", got)
	}

	if empty := Summarize(nil, 0); empty != (Summary{}) || empty.FailureRate() != 0 {
		t.Errorf("Summarize(nil) = %+v", empty)
	}
}
