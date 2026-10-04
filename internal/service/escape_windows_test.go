package service

import (
	"syscall"
	"testing"
)

func TestEscapeArgMatchesSyscall(t *testing.T) {
	for _, s := range []string{"", "--plain", "a b", `a"b`, `C:\dir\`, `C:\my dir\`, `say "hi"`, `back\\"q`, "tab\there", `\\server\share`} {
		if got, want := escapeArg(s), syscall.EscapeArg(s); got != want {
			t.Errorf("escapeArg(%q) = %s, syscall.EscapeArg = %s", s, got, want)
		}
	}
}
