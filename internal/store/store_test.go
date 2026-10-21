package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "cmdtime.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	base := time.UnixMilli(1776108000000)

	in := []Run{
		{Cmd: "make build", Start: base, Duration: 1500 * time.Millisecond, Exit: 0, Cwd: "/src/app"},
		{Cmd: "go  test ./...", Start: base.Add(time.Minute), Duration: 12 * time.Second, Exit: 1, Cwd: "/src/app"},
		{Cmd: "npm run build", Start: base.Add(2 * time.Minute), Duration: 3*time.Minute + 4*time.Second, Exit: 0, Cwd: "/src/web"},
	}
	for _, r := range in {
		if _, err := s.Add(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Query(ctx, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(in) {
		t.Fatalf("got %d runs, want %d", len(got), len(in))
	}
	for i, r := range got {
		want := in[len(in)-1-i] // newest first
		if r.Cmd != want.Cmd || !r.Start.Equal(want.Start) || r.Duration != want.Duration ||
			r.Exit != want.Exit || r.Cwd != want.Cwd {
			t.Errorf("run %d = %+v, want %+v", i, r, want)
		}
	}

	limited, err := s.Query(ctx, Filter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].Cmd != "npm run build" {
		t.Errorf("Limit 2 returned %+v", limited)
	}
}

func TestQueryFilters(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	base := time.UnixMilli(1776108000000)
	seed := []struct {
		cmd, cwd string
		d        time.Duration
	}{
		{"make build", "/src/app", time.Second},
		{"make test", "/src/app/sub", 40 * time.Second},
		{"make  build", "/src/app-old", 2 * time.Minute},
		{"make build", "/src/web", 30 * time.Second},
	}
	for i, r := range seed {
		run := Run{Cmd: r.cmd, Cwd: r.cwd, Start: base.Add(time.Duration(i) * time.Hour), Duration: r.d}
		if _, err := s.Add(ctx, run); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		f    Filter
		want int
	}{
		{"key", Filter{Key: "make build"}, 3},
		{"key is normalized", Filter{Key: " make   build"}, 3},
		{"since", Filter{Since: base.Add(2 * time.Hour)}, 2},
		{"before", Filter{Before: base.Add(2 * time.Hour)}, 2},
		{"key and since", Filter{Key: "make build", Since: base.Add(time.Hour)}, 2},
		{"cwd includes subdirectories", Filter{Cwd: "/src/app"}, 2},
		{"cwd trailing slash", Filter{Cwd: "/src/app/"}, 2},
		{"cwd root", Filter{Cwd: "/"}, 4},
		{"min duration", Filter{MinDuration: 30 * time.Second}, 3},
		{"no match", Filter{Key: "make"}, 0},
	}
	for _, tt := range tests {
		runs, err := s.Query(ctx, tt.f)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != tt.want {
			t.Errorf("%s: Query got %d runs, want %d", tt.name, len(runs), tt.want)
		}
		if n, err := s.Count(ctx, tt.f); err != nil || n != tt.want {
			t.Errorf("%s: Count = %d, %v; want %d", tt.name, n, err, tt.want)
		}
	}
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	base := time.UnixMilli(1776108000000)
	for i := 0; i < 5; i++ {
		if _, err := s.Add(ctx, Run{Cmd: "sleep 1", Start: base.Add(time.Duration(i) * 24 * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.Prune(ctx, base.Add(3*24*time.Hour))
	if err != nil || n != 3 {
		t.Fatalf("Prune = %d, %v; want 3", n, err)
	}
	if left, _ := s.Count(ctx, Filter{}); left != 2 {
		t.Errorf("%d runs left, want 2", left)
	}
}

func TestReopenKeepsSchema(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	if _, err := s.Add(ctx, Run{Cmd: "true", Start: time.Now()}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	v, err := schemaVersion(ctx, s2.db)
	if err != nil || v != len(migrations) {
		t.Fatalf("schema version = %d, %v; want %d", v, err, len(migrations))
	}
	var mode string
	if err := s2.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v; want wal", mode, err)
	}
	runs, err := s2.Query(ctx, Filter{})
	if err != nil || len(runs) != 1 {
		t.Fatalf("after reopen: %d runs, %v", len(runs), err)
	}
}

func TestConcurrentWriters(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cmdtime.db")
	const writers, each = 6, 15

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			s, err := Open(path) // separate connection, like separate shells
			if err != nil {
				errs <- err
				return
			}
			defer s.Close()
			for i := 0; i < each; i++ {
				r := Run{Cmd: fmt.Sprintf("job %d", w), Start: time.Now(), Duration: time.Second}
				if _, err := s.Add(ctx, r); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	runs, err := s.Query(ctx, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != writers*each {
		t.Errorf("got %d runs, want %d", len(runs), writers*each)
	}
}

func TestCwdSymlink(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := real + "2"
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatal(err)
	}

	s, _ := openTemp(t)
	start := time.UnixMilli(1776108000000)
	for _, r := range []Run{
		{Cmd: "make build", Cwd: link, Start: start, Duration: time.Second},
		{Cmd: "go test", Cwd: filepath.Join(link, "sub"), Start: start, Duration: time.Second},
		{Cmd: "npm ci", Cwd: sibling, Start: start, Duration: time.Second},
	} {
		if _, err := s.Add(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	// Query from somewhere else, so $PWD is not an alias of the project.
	t.Setenv("PWD", base)

	for _, dir := range []string{real, link, real + "/"} {
		runs, err := s.Query(ctx, Filter{Cwd: dir})
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != 2 {
			t.Errorf("Query %s: %d runs, want the link and its subdir", dir, len(runs))
		}
		if n, err := s.Count(ctx, Filter{Cwd: dir, Key: "make build"}); err != nil || n != 1 {
			t.Errorf("Count %s key: %d, %v; want 1", dir, n, err)
		}
	}
	if runs, err := s.Query(ctx, Filter{Cwd: sibling}); err != nil || len(runs) != 1 || runs[0].Cmd != "npm ci" {
		t.Errorf("sibling filter: %+v, %v", runs, err)
	}
}

// A database written before cwd_canon existed still matches --cwd . :
// the shell's logical $PWD is an alias of the physical working directory.
func TestLegacyCwdSymlink(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cmdtime.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO runs (key, cmd, started_at, duration_ms, exit_code, cwd) VALUES ('make build', 'make build', 1, 1000, 0, ?)`, link); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	v, err := schemaVersion(ctx, s.db)
	if err != nil || v != len(migrations) {
		t.Fatalf("schema version = %d, %v; want %d", v, err, len(migrations))
	}

	t.Setenv("PWD", link)
	runs, err := s.Query(ctx, Filter{Cwd: real})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Cwd != link {
		t.Fatalf("legacy row via physical --cwd: %+v", runs)
	}
	// Away from the directory, the physical path cannot invent the old
	// logical name. The row is still there under the path that was stored.
	t.Setenv("PWD", base)
	if n, err := s.Count(ctx, Filter{Cwd: real}); err != nil || n != 0 {
		t.Errorf("legacy row from elsewhere: count %d, %v; want 0", n, err)
	}
	if n, err := s.Count(ctx, Filter{Cwd: link}); err != nil || n != 1 {
		t.Errorf("legacy row by its stored path: count %d, %v; want 1", n, err)
	}
}
