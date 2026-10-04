// Package integration holds end-to-end tests that build and run the real
// keepalive binary. Tests that hold a real power assertion skip under -short.
package integration

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testVersion = "9.9.9-e2e"

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "keepalive-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "keepalive")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-ldflags", "-X main.version="+testVersion, "-o", binary,
		"github.com/stigoleg/keep-alive/v2/cmd/keepalive")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build keepalive: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// keepalive runs the binary with an isolated config and cache directory.
func keepalive(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(binary, args...)
	home := t.TempDir()
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"NO_COLOR=1",
	)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "KEEPALIVE_") {
			t.Fatalf("test environment sets %s; unset it", kv)
		}
	}
	return cmd
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

func TestVersion(t *testing.T) {
	for _, arg := range []string{"-v", "--version", "version"} {
		out, err := keepalive(t, arg).Output()
		if err != nil {
			t.Fatalf("%s: %v", arg, err)
		}
		if got, want := string(out), "keepalive "+testVersion+"\n"; got != want {
			t.Fatalf("%s printed %q, want %q", arg, got, want)
		}
	}
}

func TestUsageErrorsExit2(t *testing.T) {
	for _, args := range [][]string{
		{"-d", "0"},
		{"-d", "-5"},
		{"-d", "30s"},
		{"-b", "100"}, // no battery, or threshold not below the current level
		{"-b", "101"},
		{"-d", "1h", "-c", "22:00"},
		{"--nope"},
	} {
		cmd := keepalive(t, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		if code := exitCode(err); code != 2 {
			t.Errorf("%v: exit %d, want 2 (stderr %q)", args, code, stderr.String())
		}
		if !strings.HasPrefix(stderr.String(), "keepalive: error: ") {
			t.Errorf("%v: stderr %q", args, stderr.String())
		}
	}
}

func TestConfigPathFollowsXDG(t *testing.T) {
	cmd := keepalive(t, "config", "path")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var xdg string
	for _, kv := range cmd.Env {
		if v, ok := strings.CutPrefix(kv, "XDG_CONFIG_HOME="); ok {
			xdg = v
		}
	}
	if want := filepath.Join(xdg, "keepalive", "config.toml") + "\n"; string(out) != want {
		t.Fatalf("config path = %q, want %q", out, want)
	}
}

func TestCompletionZsh(t *testing.T) {
	out, err := keepalive(t, "completion", "zsh").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "#compdef keepalive") {
		t.Fatalf("not a zsh completion script:\n%.200s", out)
	}
}

func TestRootWithACommandIsUsageError(t *testing.T) {
	cmd := keepalive(t, "make", "release")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if code := exitCode(cmd.Run()); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), `hint: did you mean "keepalive run -- make release"?`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
