package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dan2756/cmdtime/internal/dur"
	"github.com/dan2756/cmdtime/internal/keys"
	"github.com/dan2756/cmdtime/internal/stats"
	"github.com/dan2756/cmdtime/internal/store"
)

const recentUsage = `Usage: cmdtime recent [flags]

Show the most recently recorded runs, newest first.

Flags:
  -n int    number of runs to show (default 20)
`

func (a *app) cmdRecent(args []string) error {
	fs := flag.NewFlagSet("recent", flag.ContinueOnError)
	n := fs.Int("n", 20, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}
	if *n <= 0 {
		return usagef("-n must be positive")
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	runs, err := st.Query(context.Background(), store.Filter{Limit: *n})
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STARTED\tDURATION\tEXIT\tDIR\tCOMMAND")
	for _, r := range runs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			r.Start.Format("2006-01-02 15:04"), dur.Format(r.Duration),
			strconv.Itoa(r.Exit), shortDir(r.Cwd), truncate(keys.Normalize(r.Cmd), 60))
	}
	return tw.Flush()
}

// shortDir abbreviates the home directory to ~.
func shortDir(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(dir, home+"/"); ok {
		return "~/" + rest
	}
	return dir
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// window is a --since value: how far back a report looks.
type window struct {
	text string        // as given, for messages: "7d"
	d    time.Duration // 0 means all time
}

func parseWindow(s string) (window, error) {
	if s == "all" {
		return window{text: s}, nil
	}
	d, err := dur.Parse(s)
	if err != nil {
		return window{}, usagef("--since: %v", err)
	}
	return window{text: s, d: d}, nil
}

// start returns the beginning of the window ending at now, or the zero
// time for all time.
func (w window) start(now time.Time) time.Time {
	if w.d == 0 {
		return time.Time{}
	}
	return now.Add(-w.d)
}

func (w window) String() string {
	if w.d == 0 {
		return "all time"
	}
	return "the last " + w.text
}

func summarize(runs []store.Run) stats.Summary {
	durs := make([]time.Duration, len(runs))
	failures := 0
	for i, r := range runs {
		durs[i] = r.Duration
		if r.Failed() {
			failures++
		}
	}
	return stats.Summarize(durs, failures)
}

func percent(f float64) string {
	return strconv.FormatFloat(f*100, 'f', 0, 64) + "%"
}

const topUsage = `Usage: cmdtime top [flags]

Show the slowest commands, ranked by 95th percentile duration.

Flags:
  --since duration   only runs from this far back, or "all" (default 7d)
  --by stat          rank by p95, p50, max, mean, total or count (default p95)
  --group-by mode    group runs by cmd, word or two-words (default cmd)
  -n int             number of commands to show (default 10)
`

var rankBy = map[string]func(stats.Summary) int64{
	"p95":   func(s stats.Summary) int64 { return int64(s.P95) },
	"p50":   func(s stats.Summary) int64 { return int64(s.P50) },
	"max":   func(s stats.Summary) int64 { return int64(s.Max) },
	"mean":  func(s stats.Summary) int64 { return int64(s.Mean) },
	"total": func(s stats.Summary) int64 { return int64(s.Total) },
	"count": func(s stats.Summary) int64 { return int64(s.Count) },
}

func (a *app) cmdTop(args []string) error {
	fs := flag.NewFlagSet("top", flag.ContinueOnError)
	since := fs.String("since", "7d", "")
	by := fs.String("by", "p95", "")
	groupBy := fs.String("group-by", "cmd", "")
	n := fs.Int("n", 10, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}
	win, err := parseWindow(*since)
	if err != nil {
		return err
	}
	rank, ok := rankBy[*by]
	if !ok {
		return usagef("--by: unknown stat %q (want p95, p50, max, mean, total or count)", *by)
	}
	mode, err := keys.ParseMode(*groupBy)
	if err != nil {
		return usagef("--group-by: %v", err)
	}
	if *n <= 0 {
		return usagef("-n must be positive")
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	runs, err := st.Query(context.Background(), store.Filter{Since: win.start(a.now())})
	if err != nil {
		return err
	}

	byKey := map[string][]store.Run{}
	for _, r := range runs {
		k := keys.Key(r.Cmd, mode)
		byKey[k] = append(byKey[k], r)
	}
	type row struct {
		key string
		sum stats.Summary
	}
	rows := make([]row, 0, len(byKey))
	for k, rs := range byKey {
		rows = append(rows, row{k, summarize(rs)})
	}
	slices.SortFunc(rows, func(x, y row) int {
		if c := cmp.Compare(rank(y.sum), rank(x.sum)); c != 0 {
			return c
		}
		return strings.Compare(x.key, y.key)
	})
	if len(rows) > *n {
		rows = rows[:*n]
	}

	tw := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "COMMAND\tRUNS\tP50\tP95\tMAX\tFAILED")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n", truncate(r.key, 50), r.sum.Count,
			dur.Format(r.sum.P50), dur.Format(r.sum.P95), dur.Format(r.sum.Max), percent(r.sum.FailureRate()))
	}
	return tw.Flush()
}

const statsUsage = `Usage: cmdtime stats COMMAND [flags]

Summarize the runs of one command: how often it ran, how long it took
(mean, median, 95th percentile, max), how often it failed, and whether
its median got faster or slower than in the period before.

Flags:
  --since duration   period to summarize, or "all" (default 30d)
  --group-by mode    match COMMAND as a whole (cmd), or only its first word
                     (word) or first two words (two-words) (default cmd)

Examples:
  cmdtime stats "make build"
  cmdtime stats npm --group-by word --since 7d
`

func (a *app) cmdStats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	since := fs.String("since", "30d", "")
	groupBy := fs.String("group-by", "cmd", "")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("want one command to summarize, e.g. cmdtime stats \"make build\"")
	}
	win, err := parseWindow(*since)
	if err != nil {
		return err
	}
	mode, err := keys.ParseMode(*groupBy)
	if err != nil {
		return usagef("--group-by: %v", err)
	}
	key := keys.Key(pos[0], mode)
	if key == "" {
		return usagef("empty command")
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	// Load the period before this one too, to compare against.
	now := a.now()
	f := store.Filter{}
	if mode == keys.Cmd {
		f.Key = key
	}
	if win.d > 0 {
		f.Since = now.Add(-2 * win.d)
	}
	runs, err := st.Query(context.Background(), f)
	if err != nil {
		return err
	}
	var cur, prev []store.Run
	for _, r := range runs {
		if keys.Key(r.Cmd, mode) != key {
			continue
		}
		if r.Start.Before(win.start(now)) {
			prev = append(prev, r)
		} else {
			cur = append(cur, r)
		}
	}
	if len(cur) == 0 {
		return exitError{code: 1, err: fmt.Errorf("no runs of %q in %s", key, win)}
	}
	s := summarize(cur)

	tw := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s: %d runs in %s\n", key, s.Count, win)
	fmt.Fprintf(tw, "  mean\t%s\n", dur.Format(s.Mean))
	fmt.Fprintf(tw, "  p50\t%s\n", dur.Format(s.P50))
	fmt.Fprintf(tw, "  p95\t%s\n", dur.Format(s.P95))
	fmt.Fprintf(tw, "  max\t%s\n", dur.Format(s.Max))
	fmt.Fprintf(tw, "  failed\t%d (%s)\n", s.Failures, percent(s.FailureRate()))
	if win.d > 0 {
		fmt.Fprintf(tw, "  trend\t%s\n", trend(s, summarize(prev), win))
	}
	return tw.Flush()
}

// trend compares the median of this period with the one before.
func trend(cur, prev stats.Summary, win window) string {
	if prev.Count == 0 {
		return fmt.Sprintf("no runs in the %s before", win.text)
	}
	if prev.P50 <= 0 {
		return fmt.Sprintf("p50 was %s in the %s before (%d runs)", dur.Format(prev.P50), win.text, prev.Count)
	}
	change := float64(cur.P50-prev.P50) / float64(prev.P50)
	word := "slower"
	if change < 0 {
		word = "faster"
	}
	if math.Abs(change) < 0.005 {
		return fmt.Sprintf("p50 unchanged from the %s before (%s, %d runs)", win.text, dur.Format(prev.P50), prev.Count)
	}
	return fmt.Sprintf("p50 %s %s than the %s before (%s, %d runs)",
		percent(math.Abs(change)), word, win.text, dur.Format(prev.P50), prev.Count)
}
