package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
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

// reportFlags are the flags shared by every report.
type reportFlags struct {
	format string
	cwd    string
}

func addReportFlags(fs *flag.FlagSet) *reportFlags {
	rf := &reportFlags{}
	fs.StringVar(&rf.format, "format", "table", "")
	fs.StringVar(&rf.cwd, "cwd", "", "")
	return rf
}

// resolve validates --format and makes --cwd absolute.
func (rf *reportFlags) resolve() (format, string, error) {
	f, err := parseFormat(rf.format)
	if err != nil {
		return "", "", err
	}
	if rf.cwd == "" {
		return f, "", nil
	}
	dir, err := filepath.Abs(rf.cwd)
	if err != nil {
		return "", "", fmt.Errorf("--cwd: %w", err)
	}
	return f, dir, nil
}

const reportFlagsUsage = `  --cwd dir          only runs in dir or its subdirectories, e.g. --cwd .
  --format fmt       table, json or csv (default table)
`

// runColumns describes a list of individual runs (recent, slow).
var runColumns = []column[store.Run]{
	{key: "id", raw: func(r store.Run) any { return r.ID }},
	{"STARTED", func(r store.Run) string { return r.Start.Format("2006-01-02 15:04") },
		"started_at", func(r store.Run) any { return jsonTime(r.Start) }},
	{"DURATION", func(r store.Run) string { return dur.Format(r.Duration) },
		"duration_ms", func(r store.Run) any { return r.Duration.Milliseconds() }},
	{"EXIT", func(r store.Run) string { return strconv.Itoa(r.Exit) },
		"exit", func(r store.Run) any { return r.Exit }},
	{"DIR", func(r store.Run) string { return shortDir(r.Cwd) },
		"cwd", func(r store.Run) any { return r.Cwd }},
	{"COMMAND", func(r store.Run) string { return truncate(keys.Normalize(r.Cmd), 60) },
		"cmd", func(r store.Run) any { return r.Cmd }},
}

const recentUsage = `Usage: cmdtime recent [flags]

Show the most recently recorded runs, newest first.

Flags:
  -n int             number of runs to show (default 20)
` + reportFlagsUsage

func (a *app) cmdRecent(args []string) error {
	fs := flag.NewFlagSet("recent", flag.ContinueOnError)
	n := fs.Int("n", 20, "")
	rf := addReportFlags(fs)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}
	if *n <= 0 {
		return usagef("-n must be positive")
	}
	f, cwd, err := rf.resolve()
	if err != nil {
		return err
	}
	runs, err := query(store.Filter{Cwd: cwd, Limit: *n})
	if err != nil {
		return err
	}
	return render(a.stdout, f, runColumns, runs)
}

const slowUsage = `Usage: cmdtime slow [flags]

List the runs that took at least --over, newest first.

Flags:
  --over duration    threshold (default 30s)
  --since duration   only runs from this far back, or "all" (default all)
  -n int             number of runs to show (default 20)
` + reportFlagsUsage

func (a *app) cmdSlow(args []string) error {
	fs := flag.NewFlagSet("slow", flag.ContinueOnError)
	over := fs.String("over", "30s", "")
	since := fs.String("since", "all", "")
	n := fs.Int("n", 20, "")
	rf := addReportFlags(fs)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}
	threshold, err := dur.Parse(*over)
	if err != nil {
		return usagef("--over: %v", err)
	}
	win, err := parseWindow(*since)
	if err != nil {
		return err
	}
	if *n <= 0 {
		return usagef("-n must be positive")
	}
	f, cwd, err := rf.resolve()
	if err != nil {
		return err
	}
	runs, err := query(store.Filter{
		Since:       win.start(a.now()),
		Cwd:         cwd,
		MinDuration: threshold,
		Limit:       *n,
	})
	if err != nil {
		return err
	}
	return render(a.stdout, f, runColumns, runs)
}

const pruneUsage = `Usage: cmdtime prune --older-than duration [flags]

Delete runs that started longer ago than --older-than.

Flags:
  --older-than duration   age of the runs to delete, e.g. 90d (required)
  --dry-run               only report how many runs would be deleted
`

func (a *app) cmdPrune(args []string) error {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	olderThan := fs.String("older-than", "", "")
	dryRun := fs.Bool("dry-run", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}
	if *olderThan == "" {
		return usagef("--older-than is required, e.g. --older-than 90d")
	}
	age, err := dur.Parse(*olderThan)
	if err != nil {
		return usagef("--older-than: %v", err)
	}
	if age <= 0 {
		return usagef("--older-than must be more than zero")
	}
	cutoff := a.now().Add(-age)

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()
	when := cutoff.Format("2006-01-02 15:04")
	if *dryRun {
		n, err := st.Count(ctx, store.Filter{Before: cutoff})
		if err != nil {
			return err
		}
		fmt.Fprintf(a.stdout, "would delete %d %s started before %s\n", n, plural(n, "run"), when)
		return nil
	}
	n, err := st.Prune(ctx, cutoff)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.stdout, "deleted %d %s started before %s\n", n, plural(int(n), "run"), when)
	return nil
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func query(f store.Filter) ([]store.Run, error) {
	st, err := openStore()
	if err != nil {
		return nil, err
	}
	defer st.Close()
	return st.Query(context.Background(), f)
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
// all is set only for the literal "all". A parsed duration of zero is an
// empty window ending at now, not all time.
type window struct {
	text string
	d    time.Duration
	all  bool
}

func parseWindow(s string) (window, error) {
	if s == "all" {
		return window{text: s, all: true}, nil
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
	if w.all {
		return time.Time{}
	}
	return now.Add(-w.d)
}

func (w window) String() string {
	if w.all {
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

func ms(d time.Duration) any { return d.Milliseconds() }

const topUsage = `Usage: cmdtime top [flags]

Show the slowest commands, ranked by 95th percentile duration.

Flags:
  --since duration   only runs from this far back, or "all" (default 7d)
  --by stat          rank by p95, p50, max, mean, total or count (default p95)
  --group-by mode    group runs by cmd, word or two-words (default cmd)
  -n int             number of commands to show (default 10)
` + reportFlagsUsage

var rankBy = map[string]func(stats.Summary) int64{
	"p95":   func(s stats.Summary) int64 { return int64(s.P95) },
	"p50":   func(s stats.Summary) int64 { return int64(s.P50) },
	"max":   func(s stats.Summary) int64 { return int64(s.Max) },
	"mean":  func(s stats.Summary) int64 { return int64(s.Mean) },
	"total": func(s stats.Summary) int64 { return int64(s.Total) },
	"count": func(s stats.Summary) int64 { return int64(s.Count) },
}

type group struct {
	key string
	sum stats.Summary
}

var groupColumns = []column[group]{
	{"COMMAND", func(g group) string { return truncate(g.key, 50) },
		"key", func(g group) any { return g.key }},
	{"RUNS", func(g group) string { return strconv.Itoa(g.sum.Count) },
		"runs", func(g group) any { return g.sum.Count }},
	{"P50", func(g group) string { return dur.Format(g.sum.P50) },
		"p50_ms", func(g group) any { return ms(g.sum.P50) }},
	{"P95", func(g group) string { return dur.Format(g.sum.P95) },
		"p95_ms", func(g group) any { return ms(g.sum.P95) }},
	{"MAX", func(g group) string { return dur.Format(g.sum.Max) },
		"max_ms", func(g group) any { return ms(g.sum.Max) }},
	{key: "mean_ms", raw: func(g group) any { return ms(g.sum.Mean) }},
	{key: "total_ms", raw: func(g group) any { return ms(g.sum.Total) }},
	{key: "failed", raw: func(g group) any { return g.sum.Failures }},
	{"FAILED", func(g group) string { return percent(g.sum.FailureRate()) },
		"fail_rate", func(g group) any { return g.sum.FailureRate() }},
}

func (a *app) cmdTop(args []string) error {
	fs := flag.NewFlagSet("top", flag.ContinueOnError)
	since := fs.String("since", "7d", "")
	by := fs.String("by", "p95", "")
	groupBy := fs.String("group-by", "cmd", "")
	n := fs.Int("n", 10, "")
	rf := addReportFlags(fs)
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
	f, cwd, err := rf.resolve()
	if err != nil {
		return err
	}

	runs, err := query(store.Filter{Since: win.start(a.now()), Cwd: cwd})
	if err != nil {
		return err
	}
	byKey := map[string][]store.Run{}
	for _, r := range runs {
		k := keys.Key(r.Cmd, mode)
		byKey[k] = append(byKey[k], r)
	}
	groups := make([]group, 0, len(byKey))
	for k, rs := range byKey {
		groups = append(groups, group{k, summarize(rs)})
	}
	slices.SortFunc(groups, func(x, y group) int {
		if c := cmp.Compare(rank(y.sum), rank(x.sum)); c != 0 {
			return c
		}
		return strings.Compare(x.key, y.key)
	})
	if len(groups) > *n {
		groups = groups[:*n]
	}
	return render(a.stdout, f, groupColumns, groups)
}

const statsUsage = `Usage: cmdtime stats COMMAND [flags]

Summarize the runs of one command: how often it ran, how long it took
(mean, median, 95th percentile, max), how often it failed, and whether
its median got faster or slower than in the period before.

Flags:
  --since duration   period to summarize, or "all" (default 30d)
  --group-by mode    match COMMAND as a whole (cmd), or only its first word
                     (word) or first two words (two-words) (default cmd)
` + reportFlagsUsage + `
Examples:
  cmdtime stats "make build"
  cmdtime stats npm --group-by word --since 7d
`

// commandStats is the result of "cmdtime stats".
type commandStats struct {
	key  string
	win  window
	cur  stats.Summary
	prev *stats.Summary // nil when there is no previous period (--since all)
}

// change is the relative change of the median from the previous period;
// ok is false if there is nothing to compare with.
func (s commandStats) change() (float64, bool) {
	if s.prev == nil || s.prev.Count == 0 || s.prev.P50 <= 0 {
		return 0, false
	}
	return float64(s.cur.P50-s.prev.P50) / float64(s.prev.P50), true
}

var statsColumns = []column[commandStats]{
	{key: "key", raw: func(s commandStats) any { return s.key }},
	{key: "since", raw: func(s commandStats) any { return s.win.text }},
	{key: "runs", raw: func(s commandStats) any { return s.cur.Count }},
	{key: "mean_ms", raw: func(s commandStats) any { return ms(s.cur.Mean) }},
	{key: "p50_ms", raw: func(s commandStats) any { return ms(s.cur.P50) }},
	{key: "p95_ms", raw: func(s commandStats) any { return ms(s.cur.P95) }},
	{key: "max_ms", raw: func(s commandStats) any { return ms(s.cur.Max) }},
	{key: "failed", raw: func(s commandStats) any { return s.cur.Failures }},
	{key: "fail_rate", raw: func(s commandStats) any { return s.cur.FailureRate() }},
	{key: "prev_runs", raw: func(s commandStats) any {
		if s.prev == nil {
			return nil
		}
		return s.prev.Count
	}},
	{key: "prev_p50_ms", raw: func(s commandStats) any {
		if s.prev == nil || s.prev.Count == 0 {
			return nil
		}
		return ms(s.prev.P50)
	}},
	{key: "p50_change", raw: func(s commandStats) any {
		if c, ok := s.change(); ok {
			return math.Round(c*1000) / 1000
		}
		return nil
	}},
}

func (a *app) cmdStats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	since := fs.String("since", "30d", "")
	groupBy := fs.String("group-by", "cmd", "")
	rf := addReportFlags(fs)
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
	f, cwd, err := rf.resolve()
	if err != nil {
		return err
	}

	// Load the period before this one too, to compare against.
	now := a.now()
	filter := store.Filter{Cwd: cwd}
	if mode == keys.Cmd {
		filter.Key = key
	}
	switch {
	case win.all:
	case win.d > 0:
		filter.Since = now.Add(-2 * win.d)
	default:
		filter.Since = win.start(now)
	}
	runs, err := query(filter)
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
	s := commandStats{key: key, win: win, cur: summarize(cur)}
	if win.d > 0 {
		p := summarize(prev)
		s.prev = &p
	}

	switch f {
	case formatJSON:
		return writeJSON(a.stdout, toObject(statsColumns, s))
	case formatCSV:
		return render(a.stdout, f, statsColumns, []commandStats{s})
	}
	tw := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s: %d %s in %s\n", key, s.cur.Count, plural(s.cur.Count, "run"), win)
	fmt.Fprintf(tw, "  mean\t%s\n", dur.Format(s.cur.Mean))
	fmt.Fprintf(tw, "  p50\t%s\n", dur.Format(s.cur.P50))
	fmt.Fprintf(tw, "  p95\t%s\n", dur.Format(s.cur.P95))
	fmt.Fprintf(tw, "  max\t%s\n", dur.Format(s.cur.Max))
	fmt.Fprintf(tw, "  failed\t%d (%s)\n", s.cur.Failures, percent(s.cur.FailureRate()))
	if s.prev != nil {
		fmt.Fprintf(tw, "  trend\t%s\n", s.trend())
	}
	return tw.Flush()
}

// trend describes how the median compares with the previous period.
func (s commandStats) trend() string {
	p := s.prev
	if p.Count == 0 {
		return fmt.Sprintf("no runs in the %s before", s.win.text)
	}
	before := fmt.Sprintf("the %s before (%s, %d %s)", s.win.text, dur.Format(p.P50), p.Count, plural(p.Count, "run"))
	change, ok := s.change()
	switch {
	case !ok:
		return "compared with " + before
	case math.Abs(change) < 0.005:
		return "p50 unchanged from " + before
	case change < 0:
		return fmt.Sprintf("p50 %s faster than %s", percent(-change), before)
	default:
		return fmt.Sprintf("p50 %s slower than %s", percent(change), before)
	}
}
