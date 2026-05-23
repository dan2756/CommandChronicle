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
		{"FOO=1 make build", "FOO=1 make build", "make", "make build"},
		{"sudo make install", "sudo make install", "make", "make install"},
		{"CGO_ENABLED=0 sudo -E go build", "CGO_ENABLED=0 sudo -E go build", "go", "go build"},
		{"time cargo build --release", "time cargo build --release", "cargo", "cargo build"},
		{"env -i HOME=/tmp npm ci", "env -i HOME=/tmp npm ci", "npm", "npm ci"},
		{"sudo", "sudo", "sudo", "sudo"},
		{"A=1 B=2", "A=1 B=2", "A=1", "A=1 B=2"},
		{"=x ls", "=x ls", "=x", "=x ls"},
		{"1X=2 ls", "1X=2 ls", "1X=2", "1X=2 ls"},
		{"git -c x=y status", "git -c x=y status", "git", "git -c"},
		{"sudo -u postgres psql -c vacuum", "sudo -u postgres psql -c vacuum", "psql", "psql -c"},
		{"sudo -Eu www-data php artisan", "sudo -Eu www-data php artisan", "php", "php artisan"},
		{"sudo -uroot make install", "sudo -uroot make install", "make", "make install"},
		{"sudo --user root make install", "sudo --user root make install", "make", "make install"},
		{"env -u HOME make build", "env -u HOME make build", "make", "make build"},
		{"time -f %e cargo build", "time -f %e cargo build", "cargo", "cargo build"},
		{"make build;make test", "make build;make test", "make", "make build"},
		{"ls|wc -l", "ls|wc -l", "ls", "ls"},
		{"make build&&make test", "make build&&make test", "make", "make build"},
		{"make 2>&1|tee log", "make 2>&1|tee log", "make", "make 2>&1"},
		{"grep 'a|b' x", "grep 'a|b' x", "grep", "grep 'a|b'"},
		{`echo a\;b`, `echo a\;b`, "echo", `echo a\;b`},
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
