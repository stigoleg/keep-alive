//go:build !windows

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunPropagatesExitCode(t *testing.T) {
	requirePower(t)
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
	requirePower(t)
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
	requirePower(t)
	cmd := keepalive(t, "run", "--", "sh", "-c", `trap 'echo got-int; exit 5' INT; echo ready; while :; do sleep 0.1; done`)
	// No controlling terminal: in a terminal's foreground group keepalive
	// leaves SIGINT to the terminal, which this test does not send.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
	requirePower(t)
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

// countSignals runs the signal counter under "keepalive run" in a new
// session: no controlling terminal, stdin /dev/null, keepalive leading its
// own process group. send delivers the signals once the counter is ready;
// the result is how many SIGINT/SIGQUIT the command saw.
func countSignals(t *testing.T, send func(pid int)) int {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := keepalive(t, "run", "--", self)
	cmd.Env = append(cmd.Env, signalCounterEnv+"=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	next := func(prefix string) string {
		t.Helper()
		select {
		case l := <-lines:
			if !strings.HasPrefix(l, prefix) {
				t.Fatalf("command printed %q, want %s…", l, prefix)
			}
			return l
		case <-time.After(10 * time.Second):
			t.Fatalf("no %q from the command within 10s", prefix)
			return ""
		}
	}
	next("ready")
	send(cmd.Process.Pid)
	next("got ")
	time.Sleep(500 * time.Millisecond) // room for a second delivery
	if err := syscall.Kill(cmd.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatalf("keepalive did not survive the signal: %v", err)
	}
	total := -1
	for total < 0 {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("the command printed no total")
			}
			if n, ok := strings.CutPrefix(l, "total "); ok {
				total, _ = strconv.Atoi(n)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the command did not get SIGTERM")
		}
	}
	if code := exitCode(cmd.Wait()); code != 0 {
		t.Fatalf("keepalive run exit %d, want the command's 0", code)
	}
	return total
}

func TestRunGroupSignalArrivesOnce(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGQUIT} {
		n := countSignals(t, func(pid int) { _ = syscall.Kill(-pid, sig) })
		if n != 1 {
			t.Errorf("%v to the process group: the command got it %d times, want 1", sig, n)
		}
	}
}

func TestRunForwardsSignalSentOnlyToKeepalive(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGQUIT} {
		n := countSignals(t, func(pid int) { _ = syscall.Kill(pid, sig) })
		if n != 1 {
			t.Errorf("%v to keepalive only: the command got it %d times, want 1", sig, n)
		}
	}
}
