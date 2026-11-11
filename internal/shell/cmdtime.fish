# cmdtime fish integration. Add to ~/.config/fish/config.fish:
#   cmdtime init fish | source

set -g __cmdtime_min_ms {{.MinMS}}

function __cmdtime_postexec --on-event fish_postexec
    set -l ec $status
    set -l cmd $argv[1]
    test -n "$cmd"; or return
    test -n "$CMD_DURATION"; and test "$CMD_DURATION" -ge $__cmdtime_min_ms; or return
    # fish only reports the duration; cmdtime works out the start time.
    # Disown only the recorder we just started. A bare `disown` removes the
    # most recently used job, which is the user's own background job when
    # cmdtime failed to start or has already exited.
    set -l prev $last_pid
    command cmdtime record --cmd "$cmd" --duration "$CMD_DURATION"ms \
        --exit $ec --cwd "$PWD" >/dev/null 2>&1 &
    if test "$last_pid" != "$prev"
        disown $last_pid 2>/dev/null
    end
end
