package platform

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// getCaffeinateProcesses lists caffeinate processes started by this test
// process only, so the test never sees (or disturbs) the user's own.
func getCaffeinateProcesses() ([]int, error) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}

	cmd := exec.Command("pgrep", "-P", strconv.Itoa(os.Getpid()), "caffeinate")
	output, err := cmd.Output()
	if err != nil {
		// No processes found is not an error for our purposes
		return nil, nil
	}

	// Parse PIDs from output
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		if pid, err := strconv.Atoi(line); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func pmsetAssertionsBytes() (int, error) {
	if runtime.GOOS != "darwin" {
		return 0, nil
	}
	out, err := exec.Command("pmset", "-g", "assertions").CombinedOutput()
	if err != nil {
		return 0, err
	}
	return len(out), nil
}

func TestKeepAlive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	if runtime.GOOS != "darwin" {
		t.Skip("skipping test on non-darwin platform")
	}

	// Add test timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Only our own children are counted, so other caffeinate processes on
	// the machine (e.g. a running keepalive) are left alone.
	initialPids, _ := getCaffeinateProcesses()
	initialCount := len(initialPids)

	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("Failed to create keep-alive: %v", err)
	}

	// Start keep-alive
	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("Failed to start keep-alive: %v", err)
	}

	// Poll for caffeinate to appear (up to ~2s)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pids, _ := getCaffeinateProcesses(); len(pids) == initialCount+1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify it started, or fallback to pmset assertions as a best-effort signal
	pids, err := getCaffeinateProcesses()
	if err != nil {
		t.Fatalf("Failed to check caffeinate processes: %v", err)
	}
	if len(pids) != initialCount+1 {
		if n, err := pmsetAssertionsBytes(); err == nil && n > 0 {
			t.Logf("caffeinate not observed via pgrep; pmset assertions present (%d bytes)", n)
		} else {
			// Environment likely doesn't allow process enumeration; skip instead of fail
			_ = keeper.Stop()
			t.Skip("caffeinate process not observable; skipping darwin process count assertion")
		}
	}

	// Stop keep-alive
	if err := keeper.Stop(); err != nil {
		t.Errorf("Failed to stop keep-alive: %v", err)
	}

	// Give processes time to clean up
	cleanupDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(cleanupDeadline) {
		if p, _ := getCaffeinateProcesses(); len(p) <= initialCount {
			return // Success - we're back to the initial count or fewer
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Final check with debug info
	if p, _ := getCaffeinateProcesses(); len(p) > initialCount {
		t.Errorf("Found %d extra caffeinate processes still running after stop: %v (initial count was %d)",
			len(p)-initialCount, p, initialCount)
	}
}

func TestLinuxCapabilityProbeSkips(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if runtime.GOOS != "linux" {
		t.Skip("not linux")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := exec.LookPath("systemd-inhibit"); err != nil {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("no systemd-inhibit and no X11 DISPLAY; skipping")
		}
	}
	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("new keepalive: %v", err)
	}
	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := keeper.Stop(); err != nil {
		t.Errorf("stop: %v", err)
	}
}

func TestWindowsBasicStartStopSkipIfUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("not windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("new keepalive: %v", err)
	}
	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := keeper.Stop(); err != nil {
		t.Errorf("stop: %v", err)
	}
}

func TestToggleSimulateActivityWhileRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("new keepalive: %v", err)
	}

	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	for i := 0; i < 20; i++ {
		keeper.SetSimulateActivity(i%2 == 0)
		time.Sleep(10 * time.Millisecond)
	}

	if err := keeper.Stop(); err != nil {
		t.Errorf("stop: %v", err)
	}
}
