# cmdtime zsh integration. Add to ~/.zshrc:
#   eval "$(cmdtime init zsh)"

zmodload zsh/datetime

typeset -g __cmdtime_min_ms={{.MinMS}}
typeset -g __cmdtime_cmd=
typeset -g __cmdtime_start=

# Sets REPLY to the current unix time in milliseconds without forking.
__cmdtime_now() {
	local -a t
	t=($epochtime)
	REPLY=$(( t[1] * 1000 + t[2] / 1000000 ))
}

__cmdtime_preexec() {
	__cmdtime_cmd=${1:-$3}
	__cmdtime_now
	__cmdtime_start=$REPLY
}

__cmdtime_precmd() {
	local ec=$?
	[[ -n $__cmdtime_start ]] || return 0
	local start=$__cmdtime_start
	__cmdtime_start=
	__cmdtime_now
	if (( REPLY - start >= __cmdtime_min_ms )); then
		# &! disowns the job, so the prompt never waits and no job notice is printed.
		command cmdtime record --cmd "$__cmdtime_cmd" --start $start --end $REPLY \
			--exit $ec --cwd "$PWD" >/dev/null 2>&1 &!
	fi
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec __cmdtime_preexec
# Run before any other precmd hook so $? is still the command's status.
precmd_functions=(__cmdtime_precmd ${precmd_functions:#__cmdtime_precmd})
