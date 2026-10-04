//go:build !windows

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// instance is a headless keepalive started in the background.
type instance struct {
	cmd   *exec.Cmd
	lines chan string
	done  chan int
}

func startInstance(t *testing.T, args ...string) *instance {
	t.Helper()
	cmd := keepalive(t, append([]string{"--json"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	in := &instance{cmd: cmd, lines: make(chan string, 256), done: make(chan int, 1)}
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-in.done })
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			in.lines <- sc.Text()
		}
		close(in.lines)
		_, _ = io.Copy(io.Discard, stdout)
		in.done <- exitCode(cmd.Wait())
		close(in.done)
	}()
	in.waitFor(t, `"type":"started"`)
	return in
}

func (in *instance) waitFor(t *testing.T, marker string) string {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case l, ok := <-in.lines:
			if !ok {
				t.Fatalf("instance exited before %q", marker)
			}
			if strings.Contains(l, marker) {
				return l
			}
		case <-timeout:
			t.Fatalf("no %q within 10s", marker)
		}
	}
}

func (in *instance) exit(t *testing.T) int {
	t.Helper()
	select {
	case code := <-in.done:
		return code
	case <-time.After(15 * time.Second):
		t.Fatal("instance did not exit")
		return -1
	}
}

func control(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := keepalive(t, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := exitCode(cmd.Run())
	return stdout.String(), stderr.String(), code
}

func TestControlARunningInstance(t *testing.T) {
	if testing.Short() {
		t.Skip("holds a real power assertion")
	}
	if _, _, code := control(t, "status"); code != 3 {
		t.Fatalf("status without an instance: exit %d, want 3", code)
	}
	// The idle threshold is out of reach, so "active on" never moves the
	// pointer.
	in := startInstance(t, "-d", "30m", "--active-idle", "10000h")

	out, errOut, code := control(t, "status", "--json")
	if code != 0 {
		t.Fatalf("status --json: exit %d: %s", code, errOut)
	}
	var st struct {
		PID      int    `json:"pid"`
		Origin   string `json:"origin"`
		Version  string `json:"version"`
		Snapshot struct {
			Running   bool   `json:"running"`
			Mode      string `json:"mode"`
			Remaining int64  `json:"remaining"`
			PowerHold string `json:"power_hold"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("status --json: %v: %s", err, out)
	}
	if st.PID != in.cmd.Process.Pid || st.Origin != "terminal" || st.Version != testVersion || !st.Snapshot.Running ||
		st.Snapshot.Mode != "duration" || st.Snapshot.Remaining <= 0 || st.Snapshot.PowerHold == "" {
		t.Fatalf("status = %+v", st)
	}

	if out, errOut, code := control(t, "status"); code != 0 || !strings.Contains(out, "left (until ") {
		t.Fatalf("status: exit %d: %s%s", code, out, errOut)
	}
	if out, errOut, code := control(t, "active", "on"); code != 0 || out != "activity simulation on\n" {
		t.Fatalf("active on: exit %d: %q %s", code, out, errOut)
	}
	in.waitFor(t, `"active":true`)
	if out, errOut, code := control(t, "active", "off"); code != 0 || out != "activity simulation off\n" {
		t.Fatalf("active off: exit %d: %q %s", code, out, errOut)
	}
	if out, errOut, code := control(t, "extend", "10m"); code != 0 || !strings.Contains(out, "(40m left)") && !strings.Contains(out, "(39m") {
		t.Fatalf("extend: exit %d: %q %s", code, out, errOut)
	}
	if out, errOut, code := control(t, "stop"); code != 0 || !strings.HasPrefix(out, "stopped keepalive (pid ") {
		t.Fatalf("stop: exit %d: %q %s", code, out, errOut)
	}
	last := in.waitFor(t, `"type":"stopped"`)
	if !strings.Contains(last, `"reason":"ipc"`) {
		t.Fatalf("stopped event = %s", last)
	}
	if code := in.exit(t); code != 0 {
		t.Fatalf("instance exit %d", code)
	}
	if _, _, code := control(t, "status"); code != 3 {
		t.Fatalf("status after stop: exit %d, want 3", code)
	}
}

func TestSecondInstanceRefusedAndReplace(t *testing.T) {
	if testing.Short() {
		t.Skip("holds a real power assertion")
	}
	first := startInstance(t, "-d", "30m")

	_, errOut, code := control(t, "--plain", "-d", "30m")
	if code != 1 {
		t.Fatalf("second instance: exit %d, want 1: %s", code, errOut)
	}
	if !strings.Contains(errOut, "keepalive: error: another keepalive is already running (pid ") ||
		!strings.Contains(errOut, "started by terminal)") ||
		!strings.Contains(errOut, `hint: use "keepalive status", "keepalive stop", or start with --replace`) {
		t.Fatalf("stderr = %q", errOut)
	}

	// keepalive run does not need the lock.
	if _, errOut, code := control(t, "run", "--", "true"); code != 0 {
		t.Fatalf("run next to an instance: exit %d: %s", code, errOut)
	}

	second := startInstance(t, "--replace", "-d", "30m")
	if code := first.exit(t); code != 0 {
		t.Fatalf("replaced instance exit %d", code)
	}
	out, _, code := control(t, "status", "--json")
	if code != 0 || !strings.Contains(out, `"pid":`+itoa(second.cmd.Process.Pid)) {
		t.Fatalf("status after replace: exit %d: %s", code, out)
	}
	if _, _, code := control(t, "stop"); code != 0 {
		t.Fatalf("stop: exit %d", code)
	}
	if code := second.exit(t); code != 0 {
		t.Fatalf("second instance exit %d", code)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
