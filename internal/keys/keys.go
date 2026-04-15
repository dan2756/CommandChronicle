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

// Key returns the group cmd belongs to under mode m.
func Key(cmd string, m Mode) string {
	if m == Cmd {
		return Normalize(cmd)
	}
	n := 1
	if m == TwoWords {
		n = 2
	}
	var words []string
	for _, f := range strings.Fields(cmd) {
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
