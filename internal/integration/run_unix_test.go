//go:build !windows

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunPropagatesExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("holds a real power assertion")
	}
	for script, want := range map[string]int{
		"exit 0":        0,
		"exit 7":        7,
		"kill -TERM $$": 143,
	} {
		cmd := keepalive(t, "run", "--", "sh", "-c", script)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if code := exitCode(cmd.Run()); code != want {
			t.Errorf("%q: exit %d, want %d (stderr %q)", script, code, want, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("%q: keepalive wrote to stdout: %q", script, stdout.String())
		}
		if !strings.HasPrefix(stderr.String(), "keepalive: keeping system and display awake while sh runs") {
			t.Errorf("%q: stderr %q", script, stderr.String())
		}
		if strings.Count(stderr.String(), "\n") != 1 {
			t.Errorf("%q: more than the start line on stderr: %q", script, stderr.String())
		}
	}
}

func TestRunChildKeepsStdout(t *testing.T) {
	cmd := keepalive(t, "run", "--json", "--", "sh", "-c", "echo hello; echo oops >&2")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %s", err, stderr.String())
	}
	if stdout.String() != "hello\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	var types []string
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if line == "oops" {
			continue
		}
		var ev event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("stderr line %q: %v", line, err)
		}
		types = append(types, ev.Type)
		if ev.Type == "stopped" && ev.Reason != "command_exited" {
			t.Fatalf("stopped reason = %q", ev.Reason)
		}
	}
	if types[0] != "started" || types[len(types)-1] != "stopped" {
		t.Fatalf("event types = %v", types)
	}
}

func TestRunForwardsSIGINT(t *testing.T) {
	if testing.Short() {
		t.Skip("holds a real power assertion")
	}
	cmd := keepalive(t, "run", "--", "sh", "-c", `trap 'echo got-int; exit 5' INT; echo ready; while :; do sleep 0.1; done`)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	sc := bufio.NewScanner(stdout)
	if !sc.Scan() || sc.Text() != "ready" {
		t.Fatalf("first line %q", sc.Text())
	}
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if !sc.Scan() || sc.Text() != "got-int" {
		t.Fatalf("child did not get SIGINT: %q", sc.Text())
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if code := exitCode(err); code != 5 {
			t.Fatalf("exit %d, want 5", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("keepalive did not exit after the child")
	}
}

func TestRunCommandNotFound(t *testing.T) {
	cmd := keepalive(t, "run", "--", "no-such-command-keepalive-e2e")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if code := exitCode(cmd.Run()); code != 127 {
		t.Fatalf("exit %d, want 127 (stderr %q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "command not found: no-such-command-keepalive-e2e") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWatchedPIDExit(t *testing.T) {
	if testing.Short() {
		t.Skip("holds a real power assertion")
	}
	sleeper := keepalive(t) // only for its environment
	sleeper.Path, sleeper.Args = "/bin/sleep", []string{"sleep", "2"}
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	go sleeper.Wait()

	cmd := keepalive(t, "--json", "--pid", strconv.Itoa(sleeper.Process.Pid))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("exit: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	var last event
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatal(err)
	}
	want := "process " + strconv.Itoa(sleeper.Process.Pid) + " exited"
	if last.Type != "stopped" || last.Reason != "process_exited" || !strings.Contains(last.Message, want) {
		t.Fatalf("last event = %+v", last)
	}
}

func TestWhileMissingProcessIsUsageError(t *testing.T) {
	cmd := keepalive(t, "--plain", "--while", "no-such-process-keepalive-e2e")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if code := exitCode(cmd.Run()); code != 2 {
		t.Fatalf("exit %d, want 2 (stderr %q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "hint: start the app first") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
