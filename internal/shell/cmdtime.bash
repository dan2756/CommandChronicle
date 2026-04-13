# cmdtime bash integration. Add to the end of ~/.bashrc:
#   eval "$(cmdtime init bash)"

__cmdtime_min_ms={{.MinMS}}
__cmdtime_armed=
__cmdtime_start=
__cmdtime_cmd=

# Sets __cmdtime_t to the current unix time in milliseconds. Bash 5 has
# EPOCHREALTIME; older versions (macOS /bin/bash) fall back to seconds.
__cmdtime_now() {
	if [[ -n ${EPOCHREALTIME-} ]]; then
		local t=${EPOCHREALTIME/[.,]/}
		__cmdtime_t=$((t / 1000))
	else
		__cmdtime_t=$(($(date +%s) * 1000))
	fi
}

# DEBUG trap: runs before every simple command. Only the first command
# after a prompt is timed; the prompt's own commands and completion
# functions are ignored.
__cmdtime_preexec() {
	[[ -n $__cmdtime_armed ]] || return 0
	[[ -n ${COMP_LINE-} || -n ${READLINE_LINE-} ]] && return 0
	case $BASH_COMMAND in __cmdtime_*) return 0 ;; esac
	__cmdtime_armed=

	local line re='^[[:space:]]*[0-9]+[*[:space:]][[:space:]]*(.*)$'
	line=$(HISTTIMEFORMAT= builtin history 1)
	if [[ $line =~ $re ]]; then
		__cmdtime_cmd=${BASH_REMATCH[1]}
	else
		__cmdtime_cmd=$BASH_COMMAND
	fi
	__cmdtime_now
	__cmdtime_start=$__cmdtime_t
}

# First entry of PROMPT_COMMAND: record the command that just finished.
__cmdtime_precmd() {
	local ec=$?
	__cmdtime_armed=
	if [[ -n $__cmdtime_start ]]; then
		local start=$__cmdtime_start
		__cmdtime_start=
		__cmdtime_now
		if ((__cmdtime_t - start >= __cmdtime_min_ms)); then
			# Backgrounded inside a subshell: the prompt never waits for it
			# and it stays out of the job table (no "[1] 1234" notices).
			(command cmdtime record --cmd "$__cmdtime_cmd" --start "$start" \
				--end "$__cmdtime_t" --exit "$ec" --cwd "$PWD" >/dev/null 2>&1 &)
		fi
	fi
	return $ec
}

# Last entry of PROMPT_COMMAND: the next command typed is the one to time.
__cmdtime_arm() {
	local ec=$?
	__cmdtime_armed=1
	return $ec
}

if [[ ${PROMPT_COMMAND-} != *__cmdtime_precmd* ]]; then
	PROMPT_COMMAND=__cmdtime_precmd$'\n'${PROMPT_COMMAND:+$PROMPT_COMMAND$'\n'}__cmdtime_arm
	trap '__cmdtime_preexec' DEBUG
fi
