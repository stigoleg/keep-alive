//go:build !windows

package integration

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

type event struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Reason   string `json:"reason"`
	Snapshot struct {
		Running bool   `json:"running"`
		Mode    string `json:"mode"`
	} `json:"snapshot"`
}

// runUntilStarted starts the binary, waits until it prints a line containing
// marker, then sends sig and returns all stdout and the exit code.
func runUntilStarted(t *testing.T, sig os.Signal, marker string, args ...string) ([]string, int) {
	t.Helper()
	cmd := keepalive(t, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()

	var out []string
	timeout := time.After(10 * time.Second)
wait:
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("exited before %q: %v", marker, out)
			}
			out = append(out, l)
			if strings.Contains(l, marker) {
				break wait
			}
		case <-timeout:
			t.Fatalf("no %q within 10s: %v", marker, out)
		}
	}

	time.Sleep(time.Second) // let it run for a moment
	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	for l := range lines {
		out = append(out, l)
	}
	_, _ = io.Copy(io.Discard, stdout)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return out, exitCode(err)
	case <-time.After(10 * time.Second):
		t.Fatal("did not exit within 10s of the signal")
		return nil, -1
	}
}

func TestHeadlessJSONStopsOnSIGTERM(t *testing.T) {
	requirePower(t)
	lines, code := runUntilStarted(t, syscall.SIGTERM, `"type":"started"`, "--plain", "--json", "-d", "1m")
	if code != 0 {
		t.Fatalf("exit %d, want 0; output %v", code, lines)
	}
	var events []event
	for _, l := range lines {
		var ev event
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Fatalf("invalid NDJSON line %q: %v", l, err)
		}
		events = append(events, ev)
	}
	first, last := events[0], events[len(events)-1]
	if first.Type != "started" || !first.Snapshot.Running || first.Snapshot.Mode != "duration" {
		t.Fatalf("first event = %+v", first)
	}
	if last.Type != "stopped" || last.Reason != "signal" || last.Snapshot.Running {
		t.Fatalf("last event = %+v", last)
	}
}

func TestHeadlessHumanStopsOnSIGINT(t *testing.T) {
	requirePower(t)
	lines, code := runUntilStarted(t, syscall.SIGINT, "keeping system", "--plain", "-d", "1m")
	if code != 0 {
		t.Fatalf("exit %d, want 0; output %v", code, lines)
	}
	if last := lines[len(lines)-1]; !strings.HasSuffix(last, "stopped: interrupted") {
		t.Fatalf("last line = %q", last)
	}
	if !strings.Contains(lines[0], "keeping system and display awake for 1m") {
		t.Fatalf("first line = %q", lines[0])
	}
}
