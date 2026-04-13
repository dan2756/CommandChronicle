package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dan2756/cmdtime/internal/dur"
	"github.com/dan2756/cmdtime/internal/keys"
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

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
