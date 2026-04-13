// Package keys turns command lines into the keys runs are grouped by.
package keys

import "strings"

// Normalize collapses runs of whitespace so that "make  build" and
// "make build" are treated as the same command.
func Normalize(cmd string) string {
	return strings.Join(strings.Fields(cmd), " ")
}
