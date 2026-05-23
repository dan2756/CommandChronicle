package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dan2756/cmdtime/internal/store"
)

var testNow = time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

// TestMain lets a test run the test binary as cmdtime itself, by setting
// CMDTIME_TEST_MAIN=1.
func TestMain(m *testing.M) {
	if os.Getenv("CMDTIME_TEST_MAIN") == "1" {
		main()
	}
	os.Exit(m.Run())
}

// testApp points CMDTIME_DB at a fresh database filled with runs and
// returns an app whose clock is fixed at testNow.
func testApp(t *testing.T, runs ...store.Run) (*app, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cmdtime.db")
	t.Setenv("CMDTIME_DB", path)
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, r := range runs {
		if _, err := st.Add(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	return &app{stdout: &stdout, stderr: &stderr, now: func() time.Time { return testNow }}, &stdout, &stderr
}

func ago(d time.Duration) time.Time { return testNow.Add(-d) }

func run(cmd string, start time.Time, d time.Duration, exit int) store.Run {
	return store.Run{Cmd: cmd, Start: start, Duration: d, Exit: exit, Cwd: "/src/app"}
}

func TestStats(t *testing.T) {
	day := 24 * time.Hour
	a, stdout, stderr := testApp(t,
		run("make build", ago(1*day), 10*time.Second, 0),
		run("make build", ago(2*day), 20*time.Second, 1),
		run("make  build", ago(3*day), 30*time.Second, 0),
		run("make build", ago(4*day), 40*time.Second, 0),
		run("make test", ago(1*day), time.Minute, 0),
		// the week before, to compare against
		run("make build", ago(8*day), 50*time.Second, 0),
		run("make build", ago(9*day), 50*time.Second, 0),
	)
	if code := a.main([]string{"stats", "make build", "--since", "7d"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	out := stdout.String()
	for _, want := range []string{
		"make build: 4 runs in the last 7d",
		"mean    25.0s",
		"p50     25.0s",
		"p95     38.5s",
		"max     40.0s",
		"failed  1 (25%)",
		"trend   p50 50% faster than the 7d before (50.0s, 2 runs)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stats output missing %q:\n%s", want, out)
		}
	}

	stdout.Reset()
	if code := a.main([]string{"stats", "make", "--group-by", "word", "--since", "all"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if out := stdout.String(); !strings.Contains(out, "make: 7 runs in all time") || strings.Contains(out, "trend") {
		t.Errorf("stats --group-by word:\n%s", out)
	}

	if code := a.main([]string{"stats", "cargo build"}); code != 1 {
		t.Errorf("stats with no runs: exit %d, want 1", code)
	}
}

func TestTop(t *testing.T) {
	a, stdout, stderr := testApp(t,
		run("npm run build", ago(time.Hour), 90*time.Second, 0),
		run("npm run lint", ago(time.Hour), 5*time.Second, 1),
		run("go test ./...", ago(2*time.Hour), 30*time.Second, 0),
		run("go test ./...", ago(3*time.Hour), 40*time.Second, 0),
		run("old", ago(30*24*time.Hour), time.Hour, 0),
	)
	if code := a.main([]string{"top"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + 3 rows:\n%s", stdout)
	}
	for i, prefix := range []string{"COMMAND", "npm run build", "go test ./...", "npm run lint"} {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], prefix)
		}
	}

	stdout.Reset()
	if code := a.main([]string{"top", "--group-by", "word", "--by", "count", "-n", "1"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	lines = strings.Split(strings.TrimSpace(stdout.String()), "\n")
	// npm and go both ran twice; ties are broken alphabetically.
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "go ") || !strings.Contains(lines[1], " 2 ") {
		t.Errorf("top --group-by word --by count -n 1:\n%s", stdout)
	}
}

func TestUsageErrors(t *testing.T) {
	a, _, stderr := testApp(t)
	for _, args := range [][]string{
		{"top", "--by", "median"},
		{"top", "--since", "forever"},
		{"stats"},
		{"stats", "make", "--group-by", "letter"},
		{"record", "--start", "1776108000000"},
		{"nosuchcommand"},
	} {
		stderr.Reset()
		if code := a.main(args); code != 2 {
			t.Errorf("%q: exit %d, want 2 (stderr %q)", args, code, stderr)
		}
	}
}

func TestRecentFormats(t *testing.T) {
	a, stdout, stderr := testApp(t,
		run("make build", ago(2*time.Hour), 72400*time.Millisecond, 0),
		run(`echo "a, b"`, ago(time.Hour), 1500*time.Millisecond, 2),
	)

	if code := a.main([]string{"recent", "--format", "json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(got) != 2 || got[0]["cmd"] != `echo "a, b"` || got[0]["duration_ms"] != 1500.0 ||
		got[0]["exit"] != 2.0 || got[1]["started_at"] != "2026-04-15T10:00:00.000Z" {
		t.Errorf("recent --format json = %v", got)
	}
	if !strings.HasPrefix(stdout.String(), "[\n  {\n    \"id\": ") {
		t.Errorf("JSON keys not in column order:\n%s", stdout)
	}

	stdout.Reset()
	if code := a.main([]string{"recent", "--format", "csv", "-n", "1"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	recs, err := csv.NewReader(stdout).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"id", "started_at", "duration_ms", "exit", "cwd", "cmd"},
		{"2", "2026-04-15T11:00:00.000Z", "1500", "2", "/src/app", `echo "a, b"`},
	}
	if fmt.Sprint(recs) != fmt.Sprint(want) {
		t.Errorf("recent --format csv = %q, want %q", recs, want)
	}

	if code := a.main([]string{"recent", "--format", "yaml"}); code != 2 {
		t.Errorf("--format yaml: exit %d, want 2", code)
	}
}

func TestSlowAndCwd(t *testing.T) {
	a, stdout, stderr := testApp(t,
		store.Run{Cmd: "cargo build", Start: ago(time.Hour), Duration: 2 * time.Minute, Cwd: "/src/api"},
		store.Run{Cmd: "cargo test", Start: ago(2 * time.Hour), Duration: 45 * time.Second, Cwd: "/src/api/crates/db"},
		store.Run{Cmd: "npm ci", Start: ago(3 * time.Hour), Duration: 50 * time.Second, Cwd: "/src/web"},
		store.Run{Cmd: "cargo check", Start: ago(4 * time.Hour), Duration: 5 * time.Second, Cwd: "/src/api"},
	)
	if code := a.main([]string{"slow", "--over", "40s", "--cwd", "/src/api", "--format", "csv"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	recs, err := csv.NewReader(stdout).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[1][5] != "cargo build" || recs[2][5] != "cargo test" {
		t.Errorf("slow --over 40s --cwd /src/api = %q", recs)
	}

	stdout.Reset()
	if code := a.main([]string{"top", "--cwd", "/src/web", "--format", "json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var groups []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0]["key"] != "npm ci" || groups[0]["p95_ms"] != 50000.0 {
		t.Errorf("top --cwd /src/web = %v", groups)
	}
}

func TestStatsJSON(t *testing.T) {
	a, stdout, stderr := testApp(t,
		run("go test ./...", ago(time.Hour), 12*time.Second, 0),
		run("go test ./...", ago(2*time.Hour), 8*time.Second, 1),
		run("go test ./...", ago(40*24*time.Hour), 20*time.Second, 0),
	)
	if code := a.main([]string{"stats", "go test ./...", "--format", "json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	want := map[string]any{
		"key": "go test ./...", "since": "30d", "runs": 2.0, "p50_ms": 10000.0,
		"fail_rate": 0.5, "prev_runs": 1.0, "prev_p50_ms": 20000.0, "p50_change": -0.5,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("stats JSON %s = %v, want %v", k, got[k], v)
		}
	}
}

func TestPrune(t *testing.T) {
	day := 24 * time.Hour
	a, stdout, stderr := testApp(t,
		run("a", ago(100*day), time.Second, 0),
		run("b", ago(95*day), time.Second, 0),
		run("c", ago(10*day), time.Second, 0),
	)
	if code := a.main([]string{"prune", "--older-than", "90d", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := stdout.String(); got != "would delete 2 runs started before 2026-01-15 12:00\n" {
		t.Errorf("dry run printed %q", got)
	}
	stdout.Reset()
	if code := a.main([]string{"prune", "--older-than", "90d"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := stdout.String(); !strings.HasPrefix(got, "deleted 2 runs") {
		t.Errorf("prune printed %q", got)
	}
	stdout.Reset()
	a.main([]string{"recent", "--format", "csv"})
	if n := strings.Count(stdout.String(), "\n"); n != 2 {
		t.Errorf("after prune recent has %d lines, want header + 1:\n%s", n, stdout)
	}
	if code := a.main([]string{"prune"}); code != 2 {
		t.Errorf("prune without --older-than: exit %d, want 2", code)
	}
}

func TestRecord(t *testing.T) {
	a, stdout, stderr := testApp(t)
	nowMS := testNow.UnixMilli()
	ok := [][]string{
		{"record", "--cmd", "make build", "--start", fmt.Sprint(nowMS - 90000), "--end", fmt.Sprint(nowMS - 30000), "--cwd", "/src"},
		{"record", "--cmd", "fish thing", "--duration", "1500ms", "--exit", "1"},
		{"record", "--cmd", "from start", "--start", fmt.Sprint(nowMS - 10000), "--duration", "2s"},
		{"record", "--cmd", "ls", "--duration", "20ms", "--min", "1s"}, // skipped
	}
	for _, args := range ok {
		if code := a.main(args); code != 0 {
			t.Fatalf("%q: exit %d: %s", args, code, stderr)
		}
	}
	if code := a.main([]string{"record", "--cmd", "x", "--start", "1", "--end", "2", "--duration", "1s"}); code != 2 {
		t.Errorf("all of --start, --end and --duration: exit %d, want 2", code)
	}
	if code := a.main([]string{"record", "--cmd", "x", "--start", fmt.Sprint(nowMS), "--end", fmt.Sprint(nowMS - 1)}); code != 2 {
		t.Errorf("--end before --start: exit %d, want 2", code)
	}

	a.main([]string{"recent", "--format", "csv"})
	recs, err := csv.NewReader(stdout).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// id, started_at, duration_ms, exit, cwd, cmd; newest first
	want := [][]string{
		{"1", "2026-04-15T11:58:30.000Z", "60000", "0", "/src", "make build"},
		{"3", "2026-04-15T11:59:50.000Z", "2000", "0", "", "from start"},
		{"2", "2026-04-15T11:59:58.500Z", "1500", "1", "", "fish thing"},
	}
	if len(recs) != len(want)+1 {
		t.Fatalf("recorded %d runs, want %d:\n%q", len(recs)-1, len(want), recs)
	}
	slices.SortFunc(recs[1:], func(x, y []string) int { return strings.Compare(x[1], y[1]) })
	for i, w := range want {
		got := recs[i+1]
		if w[4] == "" {
			got[4] = "" // the current directory, whatever it is
		}
		if fmt.Sprint(got) != fmt.Sprint(w) {
			t.Errorf("run %d = %q, want %q", i, got, w)
		}
	}
}

func TestRun(t *testing.T) {
	a, stdout, stderr := testApp(t)
	if code := a.main([]string{"run", "--", "sh", "-c", "exit 3"}); code != 3 {
		t.Fatalf("run: exit %d, want the command's 3 (stderr %q)", code, stderr)
	}
	if code := a.main([]string{"run", "cmdtime-test-no-such-command"}); code != 127 {
		t.Errorf("run of a missing command: exit %d, want 127", code)
	}
	a.main([]string{"recent", "--format", "csv"})
	recs, _ := csv.NewReader(stdout).ReadAll()
	if len(recs) != 2 || recs[1][3] != "3" || recs[1][5] != "sh -c 'exit 3'" {
		t.Errorf("recorded %q", recs)
	}
}

// Under nohup, SIGHUP is ignored. The command must inherit that, and
// cmdtime must neither die of a hangup nor pass it on.
func TestRunKeepsIgnoredSignals(t *testing.T) {
	a, stdout, _ := testApp(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `trap '' HUP; exec "$@"`, "sh",
		exe, "run", "--", "sh", "-c", "kill -HUP $PPID $$; exit 0")
	cmd.Env = append(os.Environ(), "CMDTIME_TEST_MAIN=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cmdtime run with SIGHUP ignored: %v\n%s", err, out)
	}
	a.main([]string{"recent", "--format", "csv"})
	recs, _ := csv.NewReader(stdout).ReadAll()
	if len(recs) != 2 || recs[1][3] != "0" {
		t.Errorf("recorded %q", recs)
	}
}

// A relative --cwd is taken from the current directory, as reports do.
func TestRecordRelativeCwd(t *testing.T) {
	a, stdout, stderr := testApp(t)
	if code := a.main([]string{"record", "--cmd", "deploy", "--duration", "4m12s", "--cwd", "."}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if code := a.main([]string{"top", "--cwd", ".", "--format", "csv"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	recs, _ := csv.NewReader(stdout).ReadAll()
	if len(recs) != 2 || recs[1][0] != "deploy" {
		t.Errorf("top --cwd . after record --cwd . = %q", recs)
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"git", "commit", "-m", "it's done", "", "a/b.go"})
	if want := `git commit -m 'it'\''s done' '' a/b.go`; got != want {
		t.Errorf("shellJoin = %s, want %s", got, want)
	}
}
