# cmdtime

Logs how long your commands take to a local SQLite database and reports on them.

Did `make build` really get slower this month, or does it just feel that
way? Which commands eat most of your day? `cmdtime` hooks into your shell,
quietly records the duration and exit status of every command that takes
longer than a second, and answers those questions with percentiles instead
of gut feeling.

- zsh, bash and fish hooks; records are written in the background, so the
  prompt never waits
- `top`, `stats`, `slow` and `recent` reports with p50/p95/max, failure
  rates and period-over-period trends
- group by whole command, first word or first two words; filter by directory
- table, JSON or CSV output
- a single static binary; the database is plain SQLite

## Install

```sh
go install github.com/dan2756/cmdtime@latest
```

cmdtime uses a pure Go SQLite driver, so no C toolchain is needed and
cross-compiling (for example `GOOS=darwin`) just works.

## Shell setup

Load the hook from your shell's startup file:

```sh
# ~/.zshrc
eval "$(cmdtime init zsh)"

# ~/.bashrc (put it at the end, after other prompt tools)
eval "$(cmdtime init bash)"

# ~/.config/fish/config.fish
cmdtime init fish | source
```

Open a new shell and keep working. Each command that runs for at least a
second is handed to `cmdtime record` in the background. Faster commands
(`ls`, `cd`, `git status`) are filtered inside the shell, so cmdtime is
never started for them. Change the threshold with `--min` or `CMDTIME_MIN`:

```sh
eval "$(cmdtime init zsh --min 3s)"
```

## Reports

### top: the slowest commands

```console
$ cmdtime top
COMMAND                       RUNS  P50    P95    MAX    FAILED
cargo build --release         1     2m05s  2m05s  2m05s  0%
docker compose up --build -d  2     1m49s  2m03s  2m04s  0%
make build                    1     1m12s  1m12s  1m12s  0%
npm run build                 3     54.3s  1m08s  1m09s  0%
terraform plan                2     41.8s  45.2s  45.6s  0%
go test ./...                 2     23.4s  24.9s  25.1s  0%
git push                      2     1.9s   2.3s   2.4s   0%
```

By default it looks at the last 7 days and ranks by p95. Where did the
last month go?

```console
$ cmdtime top --since 30d --group-by word --by total
COMMAND    RUNS  P50    P95    MAX    FAILED
docker     11    1m20s  1m54s  2m04s  0%
npm        20    43.6s  55.9s  1m09s  15%
make       9     1m12s  1m26s  1m26s  0%
cargo      4     2m14s  2m35s  2m37s  0%
go         12    22.5s  27.1s  27.3s  8%
terraform  4     40.8s  45.3s  45.6s  0%
git        10    2.3s   2.9s   3.0s   0%
```

### stats: one command in detail

```console
$ cmdtime stats "make build"
make build: 9 runs in the last 30d
  mean    1m12s
  p50     1m12s
  p95     1m26s
  max     1m26s
  failed  0 (0%)
  trend   p50 14% faster than the 30d before (1m25s, 7 runs)
```

The trend compares the median with the period of the same length just
before (here, 30 to 60 days ago). With `--group-by word`, `cmdtime stats npm`
covers every `npm ...` command.

### slow: runs over a threshold

```console
$ cmdtime slow --over 2m
STARTED           DURATION  EXIT  DIR        COMMAND
2026-04-16 09:36  2m05s     0     ~/src/cli  cargo build --release
2026-04-14 22:30  2m04s     0     ~/src/api  docker compose up --build -d
2026-03-31 02:56  2m37s     0     ~/src/cli  cargo build --release
```

### recent: the latest runs

```console
$ cmdtime recent -n 3
STARTED           DURATION  EXIT  DIR        COMMAND
2026-04-16 09:36  2m05s     0     ~/src/cli  cargo build --release
2026-04-15 17:13  45.6s     0     ~/infra    terraform plan
2026-04-15 11:23  25.1s     1     ~/src/api  go test ./...
```

### prune: forget old runs

```console
$ cmdtime prune --older-than 90d
deleted 412 runs started before 2026-01-16 18:16
```

Use `--dry-run` to see the count without deleting anything.

## Grouping, filtering and output

`--group-by` (on `top` and `stats`) decides which runs count as the same
command:

| mode        | `npm run build -- --prod` is grouped as |
|-------------|------------------------------------------|
| `cmd`       | `npm run build -- --prod` (default)      |
| `two-words` | `npm run`                                |
| `word`      | `npm`                                    |

Whitespace is always normalized. With `word` and `two-words`, a line such
as `make build && make test` is grouped by its first command, and leading
variable assignments and wrappers like `sudo`, `time`, `env` or `nohup`
are skipped along with their options: `CGO_ENABLED=0 sudo -E go build`
counts as `go` / `go build`, and `sudo -u postgres psql` as `psql`.

`--cwd DIR` (on every report) keeps only runs in `DIR` or below it, so
`cmdtime top --cwd .` shows the slowest commands of the current project.

`--format json` and `--format csv` print raw values (durations in
milliseconds, times in UTC) for scripts and spreadsheets:

```console
$ cmdtime stats "make build" --format json
{
  "key": "make build",
  "since": "30d",
  "runs": 9,
  "mean_ms": 71860,
  "p50_ms": 72317,
  "p95_ms": 85633,
  "max_ms": 86021,
  "failed": 0,
  "fail_rate": 0,
  "prev_runs": 7,
  "prev_p50_ms": 84519,
  "p50_change": -0.144
}
```

Durations in flags accept `500ms`, `30s`, `5m`, `2h`, `7d`, `2w` and
combinations such as `1h30m`; a bare number means seconds. `--since all`
looks at every run.

## Recording without the hook

Wrap a single command. Input and output pass straight through, and cmdtime
exits with the command's status:

```sh
cmdtime run -- make -j8 build
```

Or record a run you timed some other way (times are unix milliseconds):

```sh
cmdtime record --cmd "make build" --start 1776108000000 --end 1776108072400 --exit 0
cmdtime record --cmd "deploy" --duration 4m12s --exit 1 --cwd ~/src/api
```

## Commands

| command | what it does |
|---------|--------------|
| `init <zsh\|bash\|fish> [--min D]` | print the shell hook |
| `record --cmd C (--start MS \| --duration D) [--end MS] [--exit N] [--cwd DIR] [--min D]` | store one run |
| `run [--] CMD [ARGS...]` | run a command and record it |
| `recent [-n 20]` | latest runs |
| `top [--since 7d] [--by p95\|p50\|max\|mean\|total\|count] [--group-by M] [-n 10]` | slowest commands |
| `stats CMD [--since 30d] [--group-by M]` | one command in detail |
| `slow [--over 30s] [--since all] [-n 20]` | runs over a threshold |
| `prune --older-than D [--dry-run]` | delete old runs |

Reports also take `--cwd DIR` and `--format table|json|csv`. Every command
has `-h`; `cmdtime --version` prints the version.

## Environment

| variable | meaning |
|----------|---------|
| `CMDTIME_DB` | database file (default `$XDG_DATA_HOME/cmdtime/cmdtime.db`, or `~/.local/share/cmdtime/cmdtime.db`) |
| `CMDTIME_MIN` | shortest command the shell hook records (default `1s`); read by `cmdtime init` |

## Exit codes

| code | meaning |
|------|---------|
| 0 | success |
| 1 | error, or `stats` found no matching runs |
| 2 | invalid flags or arguments |

`cmdtime run` exits with the wrapped command's status (127 if it could not
be started, 128+N if it was killed by signal N).

## How it works

The zsh hook uses `preexec`/`precmd`, the bash hook a `DEBUG` trap plus
`PROMPT_COMMAND`, and the fish hook the `fish_postexec` event. Each one
times the command in the shell itself and, only when it crossed the
threshold, starts `cmdtime record` in the background (`&!` in zsh, a
backgrounded subshell in bash, `disown` in fish).

The database is SQLite in WAL mode with a busy timeout, so any number of
shells can record at the same time. The schema is versioned and migrated
automatically. You can query it directly:

```sh
sqlite3 ~/.local/share/cmdtime/cmdtime.db 'SELECT cmd, duration_ms FROM runs ORDER BY id DESC LIMIT 5'
```

## Platform notes

- Linux and macOS are supported.
- macOS ships bash 3.2, which has no `EPOCHREALTIME`; there the bash hook
  falls back to whole-second timestamps from `date`, which it runs twice
  per command. bash 5 (e.g. from Homebrew), zsh and fish record
  milliseconds.
- The bash hook installs its own `DEBUG` trap, replacing any existing one.
  It works with `PROMPT_COMMAND` as a string or, in bash 5.1 and later, as
  an array; load it after other tools that change `PROMPT_COMMAND`.
- The bash hook reads each command line back with `history 1`, which
  costs a subshell per command. A line the history leaves out
  (`HISTCONTROL=ignorespace`, `HISTIGNORE`) is recorded as its first simple
  command only, and a line that is just a subshell, such as
  `(cd web && npm run build)`, is not recorded at all, because bash runs no
  `DEBUG` trap for it.
- The database path is the same on macOS (`~/.local/share/cmdtime`) unless
  `XDG_DATA_HOME` or `CMDTIME_DB` say otherwise.

## License

MIT
