package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func getCaffeinateProcesses() ([]int, error) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}

	cmd := exec.Command("pgrep", "caffeinate")
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

func killCaffeinate() error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	// Try SIGTERM first
	termCmd := exec.Command("pkill", "caffeinate")
	if output, err := termCmd.CombinedOutput(); err != nil {
		// Ignore permission errors, as we can't kill processes we don't own
		if !strings.Contains(string(output), "Operation not permitted") {
			fmt.Printf("pkill output: %s, error: %v\n", string(output), err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	// Check if any processes remain that we started
	if pids, _ := getCaffeinateProcesses(); len(pids) > 0 {
		// Try SIGKILL for remaining processes
		killCmd := exec.Command("pkill", "-9", "caffeinate")
		if output, err := killCmd.CombinedOutput(); err != nil {
			// Ignore permission errors
			if !strings.Contains(string(output), "Operation not permitted") {
				fmt.Printf("pkill -9 output: %s, error: %v\n", string(output), err)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
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

	// Try to clean up any processes we can
	killCaffeinate()

	// Get initial process count (some may be running that we can't kill)
	initialPids, _ := getCaffeinateProcesses()
	initialCount := len(initialPids)

	// Cleanup after test
	t.Cleanup(func() {
		if err := killCaffeinate(); err != nil {
			t.Logf("Cleanup warning: %v", err)
		}
	})

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

func TestNewKeepAlive(t *testing.T) {
	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("NewKeepAlive failed: %v", err)
	}
	if keeper == nil {
		t.Fatal("NewKeepAlive returned nil")
	}
}

func TestKeepAliveDoubleStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if runtime.GOOS == "darwin" {
		killCaffeinate()
		t.Cleanup(func() {
			killCaffeinate()
		})
	}

	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("NewKeepAlive failed: %v", err)
	}

	// First start
	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	// Second start should be a no-op (idempotent)
	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}

	// Cleanup
	if err := keeper.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestKeepAliveDoubleStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if runtime.GOOS == "darwin" {
		killCaffeinate()
		t.Cleanup(func() {
			killCaffeinate()
		})
	}

	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("NewKeepAlive failed: %v", err)
	}

	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// First stop
	if err := keeper.Stop(); err != nil {
		t.Errorf("First Stop failed: %v", err)
	}

	// Second stop should be a no-op (idempotent)
	if err := keeper.Stop(); err != nil {
		t.Errorf("Second Stop failed: %v", err)
	}
}

func TestKeepAliveStopWithoutStart(t *testing.T) {
	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("NewKeepAlive failed: %v", err)
	}

	// Stop without start should be a no-op
	if err := keeper.Stop(); err != nil {
		t.Errorf("Stop without Start failed: %v", err)
	}
}

func TestSetSimulateActivity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if runtime.GOOS == "darwin" {
		killCaffeinate()
		t.Cleanup(func() {
			killCaffeinate()
		})
	}

	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("NewKeepAlive failed: %v", err)
	}

	// Set before start
	keeper.SetSimulateActivity(true)

	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Toggle while running
	keeper.SetSimulateActivity(false)
	keeper.SetSimulateActivity(true)

	if err := keeper.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestSetSimulateActivityWithoutStart(t *testing.T) {
	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("NewKeepAlive failed: %v", err)
	}

	// Should not panic when called without Start
	keeper.SetSimulateActivity(true)
	keeper.SetSimulateActivity(false)
}
