// Package store keeps recorded command runs in a SQLite database.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/dan2756/cmdtime/internal/keys"
)

// Run is one recorded execution of a command.
type Run struct {
	ID       int64
	Cmd      string
	Start    time.Time
	Duration time.Duration
	Exit     int
	Cwd      string
}

// Failed reports whether the run exited with a non-zero status.
func (r Run) Failed() bool { return r.Exit != 0 }

// migrations[i] upgrades the schema from version i to version i+1.
// Append only; never edit an entry that has shipped.
var migrations = []string{
	`CREATE TABLE runs (
		id          INTEGER PRIMARY KEY,
		key         TEXT    NOT NULL,
		cmd         TEXT    NOT NULL,
		started_at  INTEGER NOT NULL, -- unix milliseconds
		duration_ms INTEGER NOT NULL,
		exit_code   INTEGER NOT NULL,
		cwd         TEXT    NOT NULL DEFAULT ''
	);
	CREATE INDEX runs_key_started ON runs (key, started_at);
	CREATE INDEX runs_started ON runs (started_at);`,
}

// Store is an open cmdtime database.
type Store struct {
	db *sql.DB
}

// DefaultPath returns the database location: $CMDTIME_DB if set, otherwise
// $XDG_DATA_HOME/cmdtime/cmdtime.db, falling back to ~/.local/share.
func DefaultPath() (string, error) {
	if p := os.Getenv("CMDTIME_DB"); p != "" {
		return p, nil
	}
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "cmdtime", "cmdtime.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate database: %w (set CMDTIME_DB)", err)
	}
	return filepath.Join(home, ".local", "share", "cmdtime", "cmdtime.db"), nil
}

// Open opens (creating if needed) the database at path and brings its
// schema up to date. Several processes may have the same database open:
// WAL mode and a busy timeout let concurrent shells record at once.
func Open(path string) (*Store, error) {
	if strings.Contains(path, "?") {
		return nil, fmt.Errorf("database path %q must not contain '?'", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// queryRower is satisfied by both *sql.DB and *sql.Tx.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func schemaVersion(ctx context.Context, q queryRower) (int, error) {
	var v int
	err := q.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&v)
	return v, err
}

func (s *Store) migrate(ctx context.Context) error {
	// Fast path: most opens find the schema current and need no write lock.
	if v, err := schemaVersion(ctx, s.db); err == nil && v == len(migrations) {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil) // BEGIN IMMEDIATE, see Open
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	v, err := schemaVersion(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (0)`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("schema version %d is newer than this cmdtime supports (%d); upgrade cmdtime", v, len(migrations))
	}
	for ; v < len(migrations); v++ {
		if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
			return fmt.Errorf("migrate to schema version %d: %w", v+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_version SET version = ?`, v); err != nil {
		return err
	}
	return tx.Commit()
}

// Add stores r and returns its id.
func (s *Store) Add(ctx context.Context, r Run) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (key, cmd, started_at, duration_ms, exit_code, cwd) VALUES (?, ?, ?, ?, ?, ?)`,
		keys.Normalize(r.Cmd), r.Cmd, r.Start.UnixMilli(), r.Duration.Milliseconds(), r.Exit, r.Cwd)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Filter selects runs. The zero Filter selects everything.
type Filter struct {
	Limit int // at most this many runs; 0 means no limit
}

// Query returns the runs matching f, newest first.
func (s *Store) Query(ctx context.Context, f Filter) ([]Run, error) {
	q := `SELECT id, cmd, started_at, duration_ms, exit_code, cwd FROM runs`
	var args []any
	q += ` ORDER BY started_at DESC, id DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []Run
	for rows.Next() {
		var r Run
		var start, ms int64
		if err := rows.Scan(&r.ID, &r.Cmd, &start, &ms, &r.Exit, &r.Cwd); err != nil {
			return nil, err
		}
		r.Start = time.UnixMilli(start)
		r.Duration = time.Duration(ms) * time.Millisecond
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
