package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dan2756/cmdtime/internal/dur"
	"github.com/dan2756/cmdtime/internal/shell"
)

const initUsage = `Usage: cmdtime init <zsh|bash|fish> [flags]

Print the shell hook that records every interactive command. Records are
written in the background, so the prompt is never slowed down.

  zsh   add to ~/.zshrc:                  eval "$(cmdtime init zsh)"
  bash  add to the end of ~/.bashrc:      eval "$(cmdtime init bash)"
  fish  add to ~/.config/fish/config.fish: cmdtime init fish | source

Flags:
  --min duration   don't record commands faster than this, e.g. 500ms or 5s
                   (default $CMDTIME_MIN, or 1s)
`

const defaultMin = time.Second

func (a *app) cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	minFlag := fs.String("min", "", "")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("want exactly one shell (zsh, bash or fish)")
	}

	opt := shell.Options{Min: defaultMin}
	src, val := "--min", *minFlag
	if val == "" {
		src, val = "CMDTIME_MIN", os.Getenv("CMDTIME_MIN")
	}
	if val != "" {
		if opt.Min, err = dur.Parse(val); err != nil {
			return usagef("%s: %v", src, err)
		}
	}
	script, err := shell.Script(pos[0], opt)
	if err != nil {
		return usagef("%v", err)
	}
	_, err = fmt.Fprint(a.stdout, script)
	return err
}
