// Command cmdtime records how long shell commands take and reports on them.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/dan2756/cmdtime/internal/store"
)

// version is overridden at build time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `cmdtime logs how long your shell commands take and reports on them.

Usage:
  cmdtime <command> [flags]

Commands:
  init      print the shell hook (eval "$(cmdtime init zsh)")
  record    store one finished command (used by the shell hooks)
  run       run a command and record how long it took
  recent    show the most recent runs
  top       show the slowest commands
  stats     summarize one command: percentiles, failures, trend
  slow      list runs that took longer than a threshold
  prune     delete old runs
  version   print the version

Run "cmdtime <command> -h" for the flags of each command.

Environment:
  CMDTIME_DB   database path (default $XDG_DATA_HOME/cmdtime/cmdtime.db,
               or ~/.local/share/cmdtime/cmdtime.db)
  CMDTIME_MIN  shortest command the shell hook records (default 1s)
`

type app struct {
	stdout io.Writer
	stderr io.Writer
	now    func() time.Time
}

type command struct {
	usage string
	run   func(a *app, args []string) error
}

var commands = map[string]command{
	"init":   {initUsage, (*app).cmdInit},
	"record": {recordUsage, (*app).cmdRecord},
	"run":    {runUsage, (*app).cmdRun},
	"recent": {recentUsage, (*app).cmdRecent},
	"top":    {topUsage, (*app).cmdTop},
	"stats":  {statsUsage, (*app).cmdStats},
	"slow":   {slowUsage, (*app).cmdSlow},
	"prune":  {pruneUsage, (*app).cmdPrune},
}

func main() {
	a := &app{stdout: os.Stdout, stderr: os.Stderr, now: time.Now}
	os.Exit(a.main(os.Args[1:]))
}

func (a *app) main(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.stderr, usage)
		return 2
	}
	name, rest := args[0], args[1:]
	switch name {
	case "-h", "--help", "help":
		fmt.Fprint(a.stdout, usage)
		return 0
	case "-version", "--version", "version":
		fmt.Fprintln(a.stdout, "cmdtime", getVersion())
		return 0
	}
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(a.stderr, "cmdtime: unknown command %q\nRun 'cmdtime --help' for usage.\n", name)
		return 2
	}

	err := cmd.run(a, rest)
	var uerr usageError
	var xerr exitError
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(a.stdout, cmd.usage)
		return 0
	case errors.As(err, &uerr):
		fmt.Fprintf(a.stderr, "cmdtime %s: %s\nRun 'cmdtime %s -h' for usage.\n", name, uerr.msg, name)
		return 2
	case errors.As(err, &xerr):
		if xerr.err != nil {
			fmt.Fprintf(a.stderr, "cmdtime %s: %v\n", name, xerr.err)
		}
		return xerr.code
	default:
		fmt.Fprintf(a.stderr, "cmdtime %s: %v\n", name, err)
		return 1
	}
}

func getVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// usageError reports bad flags or arguments; it exits with status 2.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return usageError{fmt.Sprintf(format, args...)}
}

// exitError makes cmdtime exit with a specific status, printing err if set.
type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("exit status %d", e.code)
}

// parseFlags parses args into fs, keeping flag's own output quiet so that
// errors and help text are printed consistently by main.
func parseFlags(fs *flag.FlagSet, args []string) error {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{err.Error()}
	}
	return nil
}

// parseArgs is parseFlags for commands that take positional arguments. Flags
// may come before or after them, as in: stats "make build" --since 7d.
// Everything after "--" is positional.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := parseFlags(fs, args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			return append(pos, rest...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

func openStore() (*store.Store, error) {
	path, err := store.DefaultPath()
	if err != nil {
		return nil, err
	}
	return store.Open(path)
}
