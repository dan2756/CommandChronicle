# cmdtime fish integration. Add to ~/.config/fish/config.fish:
#   cmdtime init fish | source

set -g __cmdtime_min_ms {{.MinMS}}

function __cmdtime_postexec --on-event fish_postexec
    set -l ec $status
    set -l cmd $argv[1]
    test -n "$cmd"; or return
    test -n "$CMD_DURATION"; and test "$CMD_DURATION" -ge $__cmdtime_min_ms; or return
    # fish only reports the duration; cmdtime works out the start time.
    # Disowning keeps fish from printing a notice when the job ends.
    command cmdtime record --cmd "$cmd" --duration "$CMD_DURATION"ms \
        --exit $ec --cwd "$PWD" >/dev/null 2>&1 &
    disown
end
