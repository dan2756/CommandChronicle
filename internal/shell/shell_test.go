package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScriptContainsHooks(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{"zsh", []string{
			"__cmdtime_min_ms=1500",
			"add-zsh-hook preexec __cmdtime_preexec",
			"precmd_functions=(__cmdtime_precmd ",
			"command cmdtime record --cmd",
			"&!",
		}},
		{"bash", []string{
			"__cmdtime_min_ms=1500",
			"trap '__cmdtime_preexec' DEBUG",
			"PROMPT_COMMAND=__cmdtime_precmd",
			"__cmdtime_arm",
			"case $BASH_COMMAND in __cmdtime_*)",
			"command cmdtime record --cmd",
			">/dev/null 2>&1 &)",
		}},
		{"fish", []string{
			"set -g __cmdtime_min_ms 1500",
			"function __cmdtime_postexec --on-event fish_postexec",
			"--duration \"$CMD_DURATION\"ms",
			"disown",
		}},
	}
	for _, tt := range tests {
		script, err := Script(tt.shell, Options{Min: 1500 * time.Millisecond})
		if err != nil {
			t.Fatalf("%s: %v", tt.shell, err)
		}
		for _, w := range tt.want {
			if !strings.Contains(script, w) {
				t.Errorf("%s script does not contain %q", tt.shell, w)
			}
		}
		if strings.Contains(script, "{{") {
			t.Errorf("%s script has unexpanded template actions", tt.shell)
		}
	}
}

func TestScriptErrors(t *testing.T) {
	if _, err := Script("tcsh", Options{}); err == nil {
		t.Error("Script(tcsh) succeeded, want error")
	}
	if _, err := Script("zsh", Options{Min: -time.Second}); err == nil {
		t.Error("negative Min accepted")
	}
}

// shellCmd returns the command line to run shell interactively without
// user startup files, or skips the test if the shell is not installed.
func shellCmd(t *testing.T, shell string) []string {
	t.Helper()
	path, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s not installed", shell)
	}
	switch shell {
	case "zsh":
		return []string{path, "-f"}
	case "bash":
		return []string{path, "--norc", "--noprofile"}
	case "fish":
		return []string{path, "--no-config"}
	}
	t.Fatalf("no command line for %s", shell)
	return nil
}

func TestScriptSyntax(t *testing.T) {
	for _, sh := range Shells {
		t.Run(sh, func(t *testing.T) {
			argv := append(shellCmd(t, sh), "-n")
			script, err := Script(sh, Options{Min: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s -n: %v\n%s", sh, err, out)
			}
		})
	}
}

// TestHookRecords drives a real interactive shell with the hook loaded and a
// stub cmdtime on PATH that logs how it was called.
func TestHookRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("starts interactive shells")
	}
	for _, sh := range Shells {
		t.Run(sh, func(t *testing.T) {
			if sh == "fish" {
				t.Skip("fish only runs fish_postexec handlers when attached to a terminal")
			}
			argv := append(shellCmd(t, sh), "-i")
			dir := t.TempDir()
			logFile := filepath.Join(dir, "calls")
			stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + logFile + "'\n"
			if err := os.WriteFile(filepath.Join(dir, "cmdtime"), []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			script, err := Script(sh, Options{Min: 0})
			if err != nil {
				t.Fatal(err)
			}
			hook := filepath.Join(dir, "hook")
			if err := os.WriteFile(hook, []byte(script), 0o644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(),
				"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"HISTFILE="+filepath.Join(dir, "history"),
				"PROMPT_COMMAND=",
			)
			// An empty line in the middle must not record anything.
			cmd.Stdin = strings.NewReader("source " + hook + "\necho hi | cat\n\nfalse\n")
			// The shell exits with the status of the final "false".
			out, err := cmd.CombinedOutput()
			if _, ok := err.(*exec.ExitError); err != nil && !ok {
				t.Fatalf("%s: %v\n%s", sh, err, out)
			}

			calls := waitForLines(t, logFile, 2)
			if len(calls) != 2 {
				t.Errorf("got %d recorder calls, want 2:\n%s", len(calls), strings.Join(calls, "\n"))
			}
			want := map[string]string{"echo hi | cat": "--exit 0", "false": "--exit 1"}
			for _, c := range calls {
				if strings.Contains(c, "__cmdtime") || strings.Contains(c, "source ") {
					t.Errorf("recorded a hook command: %s", c)
					continue
				}
				for name, exit := range want {
					if strings.HasPrefix(c, "record --cmd "+name+" --start ") && strings.Contains(c, exit) {
						delete(want, name)
					}
				}
			}
			for name := range want {
				t.Errorf("%q not recorded as expected; calls:\n%s", name, strings.Join(calls, "\n"))
			}
		})
	}
}

// waitForLines waits for the background recorders to write n lines.
func waitForLines(t *testing.T, path string, n int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, _ := os.ReadFile(path)
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(data) > 0 && len(lines) >= n {
			time.Sleep(100 * time.Millisecond) // catch any unexpected extra calls
			data, _ = os.ReadFile(path)
			return strings.Split(strings.TrimSpace(string(data)), "\n")
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d recorder calls, got:\n%s", n, data)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
