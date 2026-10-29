# cmdtime bash integration. Add to the end of ~/.bashrc:
#   eval "$(cmdtime init bash)"

__cmdtime_min_ms={{.MinMS}}
__cmdtime_armed=
__cmdtime_start=
__cmdtime_cmd=
# Newest history entry when last looked at, and the $BASH_COMMAND that ran
# it. __cmdtime_hist stays unset until the first prompt has read history.
unset __cmdtime_hist
__cmdtime_hist_bc=

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
# after a prompt is timed; the prompt's own commands, completion functions
# and bind -x widgets are ignored. The trap passes "$_" so that bash sets
# $_ back to it once the trap is done.
__cmdtime_preexec() {
	[[ -n $__cmdtime_armed ]] || return 0
	# READLINE_LINE is set, though maybe empty, while a bind -x widget runs.
	[[ -n ${COMP_LINE-} || -n ${READLINE_LINE+x} ]] && return 0
	case $BASH_COMMAND in __cmdtime_*) return 0 ;; esac
	__cmdtime_armed=

	# The whole command line comes from the history. If the newest entry is
	# still the one seen before and a different command ran it, this line
	# was not saved (HISTCONTROL=ignorespace, HISTIGNORE), so only the
	# simple command about to run is known.
	local line re='^[[:space:]]*[0-9]+[*[:space:]][[:space:]]*(.*)$'
	line=$(HISTTIMEFORMAT= builtin history 1)
	if [[ $line != "${__cmdtime_hist-}" ]]; then
		__cmdtime_hist=$line
		__cmdtime_hist_bc=$BASH_COMMAND
	fi
	if [[ $BASH_COMMAND == "$__cmdtime_hist_bc" && $line =~ $re ]]; then
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
	if [[ -z ${__cmdtime_hist+x} ]]; then
		__cmdtime_hist=$(HISTTIMEFORMAT= builtin history 1)
	fi
	return $ec
}

if [[ ${PROMPT_COMMAND[*]-} != *__cmdtime_precmd* ]]; then
	if ((BASH_VERSINFO[0] * 100 + BASH_VERSINFO[1] >= 501)); then
		# bash 5.1+ runs every array element and shows each one the original
		# $? and PIPESTATUS (older bash runs only element 0). A string is one
		# script, so the hook's first command would collapse a pipeline to its
		# overall status and a later prompt would miss a failing stage. Keep
		# the user's string as a single element instead of joining it on.
		if [[ $(declare -p PROMPT_COMMAND 2>/dev/null) == "declare -a"* ]]; then
			PROMPT_COMMAND=(__cmdtime_precmd "${PROMPT_COMMAND[@]}" __cmdtime_arm)
		else
			PROMPT_COMMAND=(__cmdtime_precmd ${PROMPT_COMMAND:+"$PROMPT_COMMAND"} __cmdtime_arm)
		fi
	else
		PROMPT_COMMAND=__cmdtime_precmd$'\n'${PROMPT_COMMAND:+$PROMPT_COMMAND$'\n'}__cmdtime_arm
	fi
	trap '__cmdtime_preexec "$_"' DEBUG
fi
