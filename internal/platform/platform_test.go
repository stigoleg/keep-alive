package platform

import (
	"context"
	"fmt"
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

	// Give caffeinate time to start
	time.Sleep(200 * time.Millisecond)

	// Check if we have exactly one more caffeinate process than we started with
	pids, err := getCaffeinateProcesses()
	if err != nil {
		t.Fatalf("Failed to check caffeinate processes: %v", err)
	}
	if len(pids) != initialCount+1 {
		t.Errorf("Expected %d caffeinate processes, found %d: %v", initialCount+1, len(pids), pids)
	}

	// Stop keep-alive
	if err := keeper.Stop(); err != nil {
		t.Errorf("Failed to stop keep-alive: %v", err)
	}

	// Give processes time to clean up
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pids, _ := getCaffeinateProcesses(); len(pids) <= initialCount {
			return // Success - we're back to the initial count or fewer
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Final check with debug info
	if pids, _ := getCaffeinateProcesses(); len(pids) > initialCount {
		t.Errorf("Found %d extra caffeinate processes still running after stop: %v (initial count was %d)",
			len(pids)-initialCount, pids, initialCount)
	}
}
