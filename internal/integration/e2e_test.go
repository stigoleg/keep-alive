// Package integration holds end-to-end tests that build and run the real
// keepalive binary. Tests that hold a real power assertion skip under -short.
package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/power"
)

const testVersion = "9.9.9-e2e"

var binary string

func TestMain(m *testing.M) {
	if os.Getenv(signalCounterEnv) == "1" {
		signalCounter()
		return
	}
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

var (
	envMu sync.Mutex
	envs  = map[*testing.T][]string{}
)

// testEnv is the environment every keepalive of one test shares: its own
// config, cache and control-socket directories, so instances see each other
// but nothing outside the test.
func testEnv(t *testing.T) []string {
	t.Helper()
	envMu.Lock()
	defer envMu.Unlock()
	if env, ok := envs[t]; ok {
		return env
	}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "KEEPALIVE_") {
			t.Fatalf("test environment sets %s; unset it", kv)
		}
	}
	home := t.TempDir()
	// Socket paths are limited to ~104 bytes, so the runtime dir is short.
	rt, err := os.MkdirTemp("", "ka")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.RemoveAll(rt)
		envMu.Lock()
		delete(envs, t)
		envMu.Unlock()
	})
	env := append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"KEEPALIVE_RUNTIME_DIR="+rt,
		"NO_COLOR=1",
	)
	envs[t] = env
	return env
}

// requirePower skips a test that holds a real power assertion under -short,
// and on a Linux machine where nothing can keep it awake (a bare container;
// test/docker/power covers Linux with a mock logind).
func requirePower(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("holds a real power assertion")
	}
	if runtime.GOOS != "linux" {
		return
	}
	for _, m := range power.Mechanisms() {
		if m.Available {
			return
		}
	}
	t.Skip("nothing can keep this machine awake (no logind, no desktop session bus)")
}

// keepalive runs the binary in the test's isolated environment.
func keepalive(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = testEnv(t)
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

func TestDoctorJSON(t *testing.T) {
	cmd := keepalive(t, "doctor", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := exitCode(cmd.Run())
	if code != 0 && code != 1 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var doc struct {
		Sections []struct {
			Title  string `json:"title"`
			Checks []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
				Detail string `json:"detail"`
				Fix    string `json:"fix"`
			} `json:"checks"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, stdout.String())
	}
	var titles []string
	failed := false
	for _, s := range doc.Sections {
		titles = append(titles, s.Title)
		if len(s.Checks) == 0 {
			t.Errorf("section %q has no checks", s.Title)
		}
		for _, c := range s.Checks {
			switch c.Status {
			case "ok", "warn":
			case "fail":
				failed = true
			default:
				t.Errorf("%s/%s: status %q", s.Title, c.Name, c.Status)
			}
			if c.Status != "ok" && c.Fix == "" && s.Title != "Activity probe" {
				t.Logf("%s/%s has no fix: %s", s.Title, c.Name, c.Detail)
			}
		}
	}
	want := "keepalive,Sleep prevention,Activity simulation,Battery,Notifications,Background service,Running instance"
	if got := strings.Join(titles, ","); got != want {
		t.Fatalf("sections = %s", got)
	}
	if failed != (code == 1) {
		t.Fatalf("exit %d with failed=%v", code, failed)
	}
}

// signalCounterEnv makes the test binary a command for "keepalive run"
// that counts the SIGINT and SIGQUIT it receives and prints the total on
// SIGTERM.
const signalCounterEnv = "KA_E2E_SIGNAL_COUNTER"

func signalCounter() {
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	time.AfterFunc(30*time.Second, func() { os.Exit(3) }) // never outlive a failed test
	fmt.Println("ready")
	n := 0
	for sig := range sigs {
		if sig == syscall.SIGTERM {
			fmt.Printf("total %d\n", n)
			os.Exit(0)
		}
		n++
		fmt.Printf("got %v\n", sig)
	}
}
