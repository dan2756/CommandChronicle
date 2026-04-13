package store

import (
	"context"
	"fmt"
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
