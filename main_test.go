package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dan2756/cmdtime/internal/store"
)

var testNow = time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

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
