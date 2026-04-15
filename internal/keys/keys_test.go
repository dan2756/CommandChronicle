package keys

import "testing"

func TestKey(t *testing.T) {
	tests := []struct {
		cmd              string
		whole, word, two string // expected keys for Cmd, Word, TwoWords
	}{
		{"npm run build", "npm run build", "npm", "npm run"},
		{"  make   build  ", "make build", "make", "make build"},
		{"ls", "ls", "ls", "ls"},
		{"go test ./... -run TestKey", "go test ./... -run TestKey", "go", "go test"},
		{"make build; make test", "make build; make test", "make", "make build"},
		{"make; make test", "make; make test", "make", "make"},
		{"ls | wc -l", "ls | wc -l", "ls", "ls"},
		{"cargo build && cargo test", "cargo build && cargo test", "cargo", "cargo build"},
		{"", "", "", ""},
	}
	for _, tt := range tests {
		for mode, want := range map[Mode]string{Cmd: tt.whole, Word: tt.word, TwoWords: tt.two} {
			if got := Key(tt.cmd, mode); got != want {
				t.Errorf("Key(%q, %v) = %q, want %q", tt.cmd, mode, got, want)
			}
		}
	}
}

func TestParseMode(t *testing.T) {
	for _, m := range []Mode{Cmd, Word, TwoWords} {
		got, err := ParseMode(m.String())
		if err != nil || got != m {
			t.Errorf("ParseMode(%q) = %v, %v", m.String(), got, err)
		}
	}
	if _, err := ParseMode("three-words"); err == nil {
		t.Error("ParseMode(three-words) succeeded")
	}
}
