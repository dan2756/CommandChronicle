// Package stats summarizes run durations.
package stats

import (
	"math"
	"slices"
	"time"
)

// Summary describes a set of runs of one command.
type Summary struct {
	Count    int
	Failures int
	Total    time.Duration
	Mean     time.Duration
	P50      time.Duration
	P95      time.Duration
	Max      time.Duration
}

// FailureRate returns the fraction of runs that failed, from 0 to 1.
func (s Summary) FailureRate() float64 {
	if s.Count == 0 {
		return 0
	}
	return float64(s.Failures) / float64(s.Count)
}

// Summarize computes a Summary of durations, of which failures runs failed.
func Summarize(durations []time.Duration, failures int) Summary {
	s := Summary{Count: len(durations), Failures: failures}
	if s.Count == 0 {
		return s
	}
	sorted := slices.Clone(durations)
	slices.Sort(sorted)
	for _, d := range sorted {
		s.Total += d
	}
	s.Mean = s.Total / time.Duration(s.Count)
	s.P50 = Percentile(sorted, 50)
	s.P95 = Percentile(sorted, 95)
	s.Max = sorted[len(sorted)-1]
	return s
}

// Percentile returns the p-th percentile (0-100) of sorted, which must be in
// ascending order. Values between ranks are linearly interpolated, so the
// 50th percentile of an even number of runs is the mean of the middle two.
func Percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	switch {
	case n == 0:
		return 0
	case p <= 0:
		return sorted[0]
	case p >= 100:
		return sorted[n-1]
	}
	rank := p / 100 * float64(n-1)
	lo := int(rank)
	if lo+1 >= n {
		return sorted[n-1]
	}
	frac := rank - float64(lo)
	return sorted[lo] + time.Duration(math.Round(frac*float64(sorted[lo+1]-sorted[lo])))
}
