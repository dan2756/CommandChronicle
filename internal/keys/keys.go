// Package keys turns command lines into the keys runs are grouped by.
package keys

import (
	"fmt"
	"slices"
	"strings"
)

// Mode selects how much of a command line identifies a group of runs.
type Mode int

const (
	Cmd      Mode = iota // the whole command: "npm run build"
	Word                 // the first word: "npm"
	TwoWords             // the first two words: "npm run"
)

var modeNames = []string{Cmd: "cmd", Word: "word", TwoWords: "two-words"}

func (m Mode) String() string {
	if int(m) < len(modeNames) {
		return modeNames[m]
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

// ParseMode parses "cmd", "word" or "two-words".
func ParseMode(s string) (Mode, error) {
	for m, name := range modeNames {
		if s == name {
			return Mode(m), nil
		}
	}
	return 0, fmt.Errorf("unknown grouping %q (want cmd, word or two-words)", s)
}

// Normalize collapses runs of whitespace so that "make  build" and
// "make build" are treated as the same command.
func Normalize(cmd string) string {
	return strings.Join(strings.Fields(cmd), " ")
}

// wrappers run the command that follows them; grouping looks past them so
// "sudo make install" is a "make" command. Each lists its options whose
// value is the next word, as postgres is in "sudo -u postgres psql".
var wrappers = map[string]string{
	"builtin": "", "command": "", "nocorrect": "", "noglob": "", "nohup": "",
	"doas": "-a -C -u",
	"env":  "-C -P -u --chdir --unset",
	"exec": "-a",
	"sudo": "-a -C -c -D -g -p -R -r -T -t -U -u --auth-type --chdir --chroot --close-from " +
		"--command-timeout --group --login-class --other-user --prompt --role --type --user",
	"time": "-f -o --format --output",
}

// takesValue reports whether option f of wrapper w is followed by its value
// as a separate word. In a group of short options such as -Eu, the first
// one that takes a value uses the rest of the word, or else the next word.
func takesValue(w, f string) bool {
	opts := strings.Fields(wrappers[w])
	if strings.HasPrefix(f, "--") {
		return slices.Contains(opts, f)
	}
	for i := 1; i < len(f); i++ {
		if slices.Contains(opts, "-"+f[i:i+1]) {
			return i == len(f)-1
		}
	}
	return false
}

// isAssignment reports whether f looks like a NAME=value prefix.
func isAssignment(f string) bool {
	name, _, ok := strings.Cut(f, "=")
	if !ok || name == "" {
		return false
	}
	for i, c := range name {
		letter := c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		digit := '0' <= c && c <= '9'
		if !letter && !(digit && i > 0) {
			return false
		}
	}
	return true
}

// skipPrefix drops leading variable assignments and wrapper commands (with
// their options), unless nothing would be left.
func skipPrefix(fields []string) []string {
	i, wrapper := 0, ""
	for i < len(fields) {
		f := fields[i]
		_, isWrapper := wrappers[f]
		switch {
		case isAssignment(f):
		case isWrapper:
			wrapper = f
		case wrapper != "" && strings.HasPrefix(f, "-"):
			if takesValue(wrapper, f) {
				i++
			}
		default:
			return fields[i:]
		}
		i++
	}
	return fields
}

// operatorIndex returns the index of the first shell operator (;, & or |)
// in f, which ends the command, as in "build;make" or "&&", or -1. Quoted
// or escaped ones and the & of a redirection such as 2>&1 don't count.
func operatorIndex(f string) int {
	var quote rune
	escaped := false
	for i, c := range f {
		switch {
		case escaped:
			escaped = false
		case c == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == ';' || c == '|' || c == '&' && (i == 0 || f[i-1] != '>' && f[i-1] != '<'):
			return i
		}
	}
	return -1
}

// Key returns the group cmd belongs to under mode m. The word modes skip
// leading assignments and wrappers: "FOO=1 sudo -E make install" is
// "make" by word and "make install" by two words.
func Key(cmd string, m Mode) string {
	if m == Cmd {
		return Normalize(cmd)
	}
	n := 1
	if m == TwoWords {
		n = 2
	}
	var words []string
	for _, f := range skipPrefix(strings.Fields(cmd)) {
		trimmed := f
		if i := operatorIndex(f); i >= 0 {
			trimmed = f[:i]
		}
		if trimmed != "" {
			words = append(words, trimmed)
		}
		if len(words) == n || trimmed != f {
			break // reached n words, or "build;" ended the command
		}
	}
	return strings.Join(words, " ")
}
