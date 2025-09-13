package integration

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/internal/keepalive"
	"github.com/stigoleg/keep-alive/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeepAliveIntegration verifies the integration between the keeper and platform layers
func TestKeepAliveIntegration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{"short_duration", 2 * time.Second},
		{"medium_duration", 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keeper := &keepalive.Keeper{}
			err := keeper.StartTimed(tt.duration)
			require.NoError(t, err, "keeper should start without error")

			// Verify the keeper is running
			assert.True(t, keeper.IsRunning(), "keeper should be running")
			assert.Greater(t, keeper.TimeRemaining(), time.Duration(0), "time remaining should be positive")

			// Let it run for a short duration
			time.Sleep(time.Second)

			// Verify it's still running
			assert.True(t, keeper.IsRunning(), "keeper should still be running")

			// Stop the keeper
			err = keeper.Stop()
			require.NoError(t, err, "keeper should stop without error")
			assert.False(t, keeper.IsRunning(), "keeper should not be running after stop")
		})
	}
}

// TestPlatformSpecificBehavior verifies platform-specific implementations
func TestPlatformSpecificBehavior(t *testing.T) {
	ka, err := platform.NewKeepAlive()
	require.NoError(t, err, "should create platform-specific keep-alive")

	// Start the keep-alive
	err = ka.Start(context.Background())
	require.NoError(t, err, "should start without error")

	// Platform-specific verification
	switch runtime.GOOS {
	case "darwin":
		assertDarwinBehavior(t)
	case "windows":
		assertWindowsBehavior(t)
	case "linux":
		assertLinuxBehavior(t)
	}

	// Stop and verify cleanup
	err = ka.Stop()
	require.NoError(t, err, "should stop without error")
}

func assertDarwinBehavior(t *testing.T) {
	cmd := exec.Command("pmset", "-g", "assertions")
	output, err := cmd.Output()
	require.NoError(t, err, "should get power management assertions")
	assert.Contains(t, string(output), "PreventUserIdleSystemSleep")
}

func assertWindowsBehavior(t *testing.T) {
	cmd := exec.Command("powercfg", "/requests")
	output, err := cmd.Output()
	require.NoError(t, err, "should get power requests")
	out := string(output)
	assert.Contains(t, out, "DISPLAY:")
	assert.Contains(t, out, "SYSTEM:")
}

func assertLinuxBehavior(t *testing.T) {
	cmd := exec.Command("systemctl", "status", "sleep.target")
	output, err := cmd.Output()
	if err == nil {
		assert.Contains(t, string(output), "inactive")
	}
}

// execCommand wraps exec.Command for testing
var execCommand = exec.Command
