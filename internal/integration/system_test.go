package integration

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/internal/keepalive"
	"github.com/stigoleg/keep-alive/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSystemSleepPrevention verifies that the system actually stays awake
func TestSystemSleepPrevention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping system test in short mode")
	}

	ka, err := platform.NewKeepAlive()
	require.NoError(t, err, "should create platform-specific keep-alive")

	// Start keep-alive
	err = ka.Start(context.Background())
	require.NoError(t, err, "should start without error")

	// Monitor system state for 20 seconds
	startTime := time.Now()
	for time.Since(startTime) < 20*time.Second {
		assertSystemActive(t)
		time.Sleep(2 * time.Second)
	}

	// Stop and verify cleanup
	err = ka.Stop()
	require.NoError(t, err, "should stop without error")
}

// TestUnexpectedTermination verifies proper cleanup on unexpected termination
func TestUnexpectedTermination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping system test in short mode")
	}

	// Start a separate process
	cmd := exec.Command(os.Args[0], "-test.run=TestKeepAliveHelper")
	cmd.Env = append(os.Environ(), "TEST_KEEPALIVE_HELPER=1")
	err := cmd.Start()
	require.NoError(t, err, "helper process should start")

	// Let it run for a few seconds
	time.Sleep(5 * time.Second)

	// Force kill the process
	err = cmd.Process.Signal(syscall.SIGKILL)
	require.NoError(t, err, "should kill helper process")

	// Wait a moment for cleanup
	time.Sleep(2 * time.Second)

	// Verify system returns to normal state
	assertSystemNormal(t)
}

// TestKeepAliveHelper is a helper function for TestUnexpectedTermination
func TestKeepAliveHelper(t *testing.T) {
	if os.Getenv("TEST_KEEPALIVE_HELPER") != "1" {
		return
	}

	ka, _ := platform.NewKeepAlive()
	ka.Start(context.Background())
	select {} // Block forever
}

// TestConcurrentInstances verifies behavior with multiple instances
func TestConcurrentInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping system test in short mode")
	}

	// Start multiple instances
	instances := make([]*keepalive.Keeper, 3)
	for i := range instances {
		keeper := &keepalive.Keeper{}
		err := keeper.StartTimed(5 * time.Second)
		require.NoError(t, err, "keeper %d should start", i)
		instances[i] = keeper
		defer keeper.Stop()
	}

	// Let them run concurrently
	time.Sleep(3 * time.Second)

	// Verify all are running
	for i, keeper := range instances {
		assert.True(t, keeper.IsRunning(), "keeper %d should be running", i)
	}

	// Stop them in reverse order
	for i := len(instances) - 1; i >= 0; i-- {
		require.NoError(t, instances[i].Stop(), "keeper %d should stop", i)
	}
}

func assertSystemActive(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pmset", "-g", "assertions")
		output, err := cmd.Output()
		require.NoError(t, err)
		assert.Contains(t, string(output), "PreventUserIdleSystemSleep")
	case "windows":
		cmd := exec.Command("powercfg", "/requests")
		output, err := cmd.Output()
		require.NoError(t, err)
		assert.Contains(t, string(output), "keep-alive")
	case "linux":
		cmd := exec.Command("systemctl", "status", "sleep.target")
		output, err := cmd.Output()
		if err == nil {
			assert.Contains(t, string(output), "inactive")
		}
	}
}

func assertSystemNormal(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pmset", "-g", "assertions")
		output, err := cmd.Output()
		require.NoError(t, err)
		assert.NotContains(t, string(output), "keep-alive")
	case "windows":
		cmd := exec.Command("powercfg", "/requests")
		output, err := cmd.Output()
		require.NoError(t, err)
		assert.NotContains(t, string(output), "keep-alive")
	case "linux":
		// On Linux, we just verify the process is gone
		cmd := exec.Command("pgrep", "-f", "keep-alive")
		err := cmd.Run()
		assert.Error(t, err, "keep-alive process should not be running")
	}
}
