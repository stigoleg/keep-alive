package integration

import (
	"bufio"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The binary holds IOKit power assertions in its own process: pmset lists
// them under its pid, with no caffeinate child, and powerd drops them when
// it exits, including on SIGKILL. Other assertions on the machine are
// ignored.

func TestPowerHoldVisibleInPmset(t *testing.T) {
	if testing.Short() {
		t.Skip("holds real power assertions")
	}
	for _, display := range []bool{true, false} {
		cmd := startSession(t, "--json", "--keep-display="+strconv.FormatBool(display))
		pid := cmd.Process.Pid
		got := waitAssertions(t, pid, func(a []string) bool { return slices.Contains(a, "PreventUserIdleSystemSleep") })
		if !slices.Contains(got, "PreventUserIdleSystemSleep") {
			t.Fatalf("keepalive (pid %d) holds %v", pid, got)
		}
		if has := slices.Contains(got, "PreventUserIdleDisplaySleep"); has != display {
			t.Fatalf("keep-display=%v: pid %d holds %v", display, pid, got)
		}
		if out, _ := exec.Command("pgrep", "-P", strconv.Itoa(pid), "caffeinate").Output(); len(out) > 0 {
			t.Fatalf("keepalive started caffeinate %s", out)
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("exit after SIGTERM: %v", err)
		}
		if got := assertionsOf(t, pid); len(got) != 0 {
			t.Fatalf("pid %d assertions after exit: %v", pid, got)
		}
	}
}

func TestPowerHoldReleasedOnSIGKILL(t *testing.T) {
	if testing.Short() {
		t.Skip("holds real power assertions")
	}
	cmd := startSession(t, "--json")
	pid := cmd.Process.Pid
	if got := waitAssertions(t, pid, func(a []string) bool { return len(a) > 0 }); len(got) == 0 {
		t.Fatalf("keepalive (pid %d) holds no assertions", pid)
	}
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	if got := waitAssertions(t, pid, func(a []string) bool { return len(a) == 0 }); len(got) != 0 {
		t.Fatalf("pid %d assertions 5s after SIGKILL: %v", pid, got)
	}
}

// startSession starts the binary and waits for its started event.
func startSession(t *testing.T, args ...string) *exec.Cmd {
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
	started := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.Contains(sc.Text(), `"type":"started"`) {
				started <- true
			}
		}
		close(started)
	}()
	select {
	case ok := <-started:
		if !ok {
			t.Fatal("exited before the started event")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no started event within 10s")
	}
	return cmd
}

var pmsetOwner = regexp.MustCompile(`^\s*pid (\d+)\([^)]*\): \[[^\]]*\] \S+ (\S+) named:`)

// assertionsOf returns the assertion types pmset lists for pid.
func assertionsOf(t *testing.T, pid int) []string {
	t.Helper()
	out, err := exec.Command("pmset", "-g", "assertions").Output()
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if m := pmsetOwner.FindStringSubmatch(line); m != nil && m[1] == strconv.Itoa(pid) {
			types = append(types, m[2])
		}
	}
	return types
}

// waitAssertions polls pmset for up to 5s until cond holds.
func waitAssertions(t *testing.T, pid int, cond func([]string) bool) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := assertionsOf(t, pid)
		if cond(got) || time.Now().After(deadline) {
			return got
		}
		time.Sleep(100 * time.Millisecond)
	}
}
