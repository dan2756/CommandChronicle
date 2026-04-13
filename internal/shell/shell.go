// Package shell generates the hook scripts printed by "cmdtime init".
package shell

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
	"time"
)

var (
	//go:embed cmdtime.zsh
	zshHook string
	//go:embed cmdtime.bash
	bashHook string
)

var hooks = map[string]string{
	"zsh":  zshHook,
	"bash": bashHook,
}

// Shells lists the supported shells.
var Shells = []string{"zsh", "bash"}

// Options configures a generated hook.
type Options struct {
	// Min is the shortest run worth recording. Faster commands are
	// filtered in the shell itself, so they cost nothing.
	Min time.Duration
}

// Script returns the hook for shell, ready to be eval'd.
func Script(shell string, opt Options) (string, error) {
	src, ok := hooks[shell]
	if !ok {
		return "", fmt.Errorf("unsupported shell %q (supported: %s)", shell, strings.Join(Shells, ", "))
	}
	if opt.Min < 0 {
		return "", fmt.Errorf("negative minimum duration %v", opt.Min)
	}
	t, err := template.New(shell).Parse(src)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = t.Execute(&b, struct{ MinMS int64 }{opt.Min.Milliseconds()})
	return b.String(), err
}
