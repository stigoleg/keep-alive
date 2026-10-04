package power

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These tests take real power assertions and check them with
// `pmset -g assertions`, looking only at pids this test started.

var pmsetOwner = regexp.MustCompile(`^\s*pid (\d+)\([^)]*\): \[[^\]]*\] \S+ (\S+) named:`)

// assertionsOf returns the assertion types pmset lists for pid.
func assertionsOf(t *testing.T, pid int) []string {
	t.Helper()
	out, err := exec.Command("pmset", "-g", "assertions").Output()
	if err != nil {
		t.Fatalf("pmset: %v", err)
	}
	var types []string
	for line := range strings.SplitSeq(string(out), "\n") {
		m := pmsetOwner.FindStringSubmatch(line)
		if m != nil && m[1] == strconv.Itoa(pid) {
			types = append(types, m[2])
		}
	}
	slices.Sort(types)
	return types
}

// waitAssertions polls pmset until cond holds for pid's assertions.
func waitAssertions(t *testing.T, pid int, timeout time.Duration, cond func([]string) bool) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := assertionsOf(t, pid)
		if cond(got) || time.Now().After(deadline) {
			return got
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func hasAll(want ...string) func([]string) bool {
	return func(got []string) bool {
		for _, w := range want {
			if !slices.Contains(got, w) {
				return false
			}
		}
		return true
	}
}

func isEmpty(got []string) bool { return len(got) == 0 }

// checkHoldTypes asserts the system-sleep assertion is held and the display
// assertion only when keepDisplay.
func checkHoldTypes(t *testing.T, pid int, keepDisplay bool) {
	t.Helper()
	got := waitAssertions(t, pid, 3*time.Second, hasAll(assertIdleSystemName))
	if !slices.Contains(got, assertIdleSystemName) {
		t.Fatalf("pid %d holds %v, want %s", pid, got, assertIdleSystemName)
	}
	if keepDisplay {
		got = waitAssertions(t, pid, 3*time.Second, hasAll(assertIdleDisplayName))
	}
	if has := slices.Contains(got, assertIdleDisplayName); has != keepDisplay {
		t.Fatalf("pid %d holds %v; display assertion present=%v, want %v", pid, got, has, keepDisplay)
	}
}

const (
	assertIdleSystemName  = "PreventUserIdleSystemSleep"
	assertIdleDisplayName = "PreventUserIdleDisplaySleep"
	helperEnv             = "KEEPALIVE_POWER_HELPER"
)

// TestHelperProcess is not a test: re-executed with helperEnv set it
// acquires a hold, prints the pid owning the assertions and blocks until it
// is killed.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		t.Skip("helper process for the kill tests")
	}
	var in Inhibitor = New()
	if mode == "caffeinate" {
		in = caffeinateInhibitor{}
	}
	h, err := in.Acquire(context.Background(), Options{KeepDisplay: true, Reason: "kill test"})
	if err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}
	owner := os.Getpid()
	if p, ok := h.(*procHold); ok {
		owner = p.cmd.Process.Pid
	}
	fmt.Println("ready", owner)
	select {}
}

// startHelper starts the helper in mode and returns it and the pid that
// owns its assertions.
func startHelper(t *testing.T, mode string) (*exec.Cmd, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("helper: %v", err)
	}
	var owner int
	if _, err := fmt.Sscanf(line, "ready %d", &owner); err != nil {
		t.Fatalf("helper said %q", line)
	}
	return cmd, owner
}

// testKilledOwnerReleases SIGKILLs a helper holding assertions and expects
// them gone within 5 s.
func testKilledOwnerReleases(t *testing.T, mode string) {
	if testing.Short() {
		t.Skip("holds real power assertions")
	}
	cmd, owner := startHelper(t, mode)
	checkHoldTypes(t, owner, true)
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	if got := waitAssertions(t, owner, 5*time.Second, isEmpty); len(got) != 0 {
		t.Fatalf("assertions of pid %d still listed 5s after SIGKILL: %v", owner, got)
	}
}
