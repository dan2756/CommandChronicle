# cmdtime

Logs how long your commands take to a local SQLite database and reports on them.

Ever wondered whether `make build` really got slower this month? `cmdtime`
keeps a record of every run so you can find out.

## Install

```sh
go install github.com/dan2756/cmdtime@latest
```

## Shell setup

Load the hook in your shell's startup file and every interactive command
that takes a second or more is recorded:

```sh
# ~/.zshrc
eval "$(cmdtime init zsh)"

# ~/.bashrc (put it at the end, after other prompt tools)
eval "$(cmdtime init bash)"
```

Records are written by a background process, so the prompt is never kept
waiting. Commands faster than the threshold (`ls`, `cd`, ...) are filtered
inside the shell and cost nothing. Change the threshold with `--min` or
`CMDTIME_MIN`:

```sh
eval "$(cmdtime init zsh --min 3s)"
```

## Usage

Wrap a command to time and record it:

```sh
cmdtime run -- make build
```

Record a run you timed yourself (times are unix milliseconds):

```sh
cmdtime record --cmd "make build" --start 1776108000000 --end 1776108072400 --exit 0
```

See what ran recently:

```sh
$ cmdtime recent
STARTED           DURATION  EXIT  DIR       COMMAND
2026-04-13 19:21  1m12s     0     ~/src/api  make build
2026-04-13 19:18  4.8s      1     ~/src/api  go test ./...
```

The database lives at `$XDG_DATA_HOME/cmdtime/cmdtime.db`
(`~/.local/share/cmdtime/cmdtime.db` by default). Set `CMDTIME_DB` to use
another file.

## License

MIT
