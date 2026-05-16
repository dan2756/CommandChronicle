// Package keys turns command lines into the keys runs are grouped by.
package keys

import (
	"fmt"
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

// shell operators that end the first command of a line
var operators = map[string]bool{"|": true, "|&": true, "||": true, "&&": true, ";": true, "&": true}

// wrappers run the command that follows them; grouping looks past them so
// "sudo make install" is a "make" command.
var wrappers = map[string]bool{
	"builtin": true, "command": true, "doas": true, "env": true, "exec": true,
	"nocorrect": true, "noglob": true, "nohup": true, "sudo": true, "time": true,
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
// their flags), unless nothing would be left.
func skipPrefix(fields []string) []string {
	i, wrapped := 0, false
	for i < len(fields) {
		f := fields[i]
		switch {
		case isAssignment(f):
		case wrappers[f]:
			wrapped = true
		case wrapped && strings.HasPrefix(f, "-"):
		default:
			return fields[i:]
		}
		i++
	}
	return fields
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
		if operators[f] {
			break
		}
		trimmed := strings.TrimRight(f, ";&|")
		if trimmed != "" {
			words = append(words, trimmed)
		}
		if len(words) == n || trimmed != f {
			break // reached n words, or "build;" ended the command
		}
	}
	return strings.Join(words, " ")
}
