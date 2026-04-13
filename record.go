package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/dan2756/cmdtime/internal/dur"
	"github.com/dan2756/cmdtime/internal/store"
)

const recordUsage = `Usage: cmdtime record --cmd CMD --start MS [flags]

Store one finished command. The shell hooks call this in the background
after every command; it is also handy for timing things by hand.

Flags:
  --cmd string    the command line (required)
  --start int     start time in unix milliseconds (required)
  --end int       end time in unix milliseconds (default: now)
  --exit int      exit status of the command (default 0)
  --cwd string    directory the command ran in (default: current directory)
  --min duration  skip the run if it took less than this, e.g. 2s
`

func (a *app) cmdRecord(args []string) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	cmd := fs.String("cmd", "", "")
	start := fs.Int64("start", 0, "")
	end := fs.Int64("end", 0, "")
	exit := fs.Int("exit", 0, "")
	cwd := fs.String("cwd", "", "")
	minDur := fs.String("min", "0", "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}
	if strings.TrimSpace(*cmd) == "" {
		return usagef("--cmd is required")
	}
	if *start == 0 {
		return usagef("--start is required")
	}
	if *start < 1e11 {
		return usagef("--start %d looks like seconds; want unix milliseconds", *start)
	}
	endMS := *end
	if endMS == 0 {
		endMS = a.now().UnixMilli()
	}
	if endMS < *start {
		return usagef("--end %d is before --start %d", endMS, *start)
	}
	threshold, err := dur.Parse(*minDur)
	if err != nil {
		return usagef("--min: %v", err)
	}

	run := store.Run{
		Cmd:      *cmd,
		Start:    time.UnixMilli(*start),
		Duration: time.Duration(endMS-*start) * time.Millisecond,
		Exit:     *exit,
		Cwd:      *cwd,
	}
	if run.Duration < threshold {
		return nil
	}
	if run.Cwd == "" {
		run.Cwd, _ = os.Getwd()
	}
	return save(run)
}

func save(run store.Run) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	_, err = st.Add(context.Background(), run)
	return err
}

const runUsage = `Usage: cmdtime run [--] COMMAND [ARGS...]

Run COMMAND, wait for it to finish and record how long it took. Input and
output are passed straight through, and cmdtime exits with the command's
exit status. Runs are recorded however short they are.

Example:
  cmdtime run -- make -j8 build
`

func (a *app) cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	argv := fs.Args()
	if len(argv) == 0 {
		return usagef("missing command to run")
	}

	c := exec.Command(argv[0], argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr

	// Ctrl-C and Ctrl-\ already reach the child through the terminal's
	// process group; catch them so cmdtime survives to record the run.
	// Signals aimed only at cmdtime are passed on.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)

	start := a.now()
	if err := c.Start(); err != nil {
		return exitError{code: 127, err: err}
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigs:
				if s == syscall.SIGTERM || s == syscall.SIGHUP {
					c.Process.Signal(s)
				}
			case <-done:
				return
			}
		}
	}()
	err := c.Wait()
	close(done)
	end := a.now()

	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			code = 128 + int(ws.Signal())
		}
	} else if err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	run := store.Run{Cmd: shellJoin(argv), Start: start, Duration: end.Sub(start), Exit: code, Cwd: cwd}
	if err := save(run); err != nil {
		fmt.Fprintf(a.stderr, "cmdtime run: not recorded: %v\n", err)
	}
	if code != 0 {
		return exitError{code: code}
	}
	return nil
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellJoin quotes argv so the stored command reads like what was typed.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, arg := range argv {
		if shellSafe.MatchString(arg) {
			parts[i] = arg
		} else {
			parts[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
	}
	return strings.Join(parts, " ")
}
