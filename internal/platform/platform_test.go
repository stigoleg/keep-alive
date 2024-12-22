package platform

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestKeepAlive(t *testing.T) {
	keeper, err := NewKeepAlive()
	if err != nil {
		t.Fatalf("Failed to create keep-alive: %v", err)
	}

	// Start keep-alive
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := keeper.Start(ctx); err != nil {
		t.Fatalf("Failed to start keep-alive: %v", err)
	}

	// Verify system state based on platform
	switch runtime.GOOS {
	case "darwin":
		time.Sleep(2 * time.Second) // Give caffeinate time to start
		cmd := exec.Command("pgrep", "caffeinate")
		if err := cmd.Run(); err != nil {
			t.Error("caffeinate process not found")
		}
	case "linux":
		time.Sleep(2 * time.Second) // Give processes time to start
		cmd := exec.Command("pgrep", "-f", "systemd-inhibit")
		if err := cmd.Run(); err != nil {
			// Check xset as fallback
			cmd = exec.Command("xset", "q")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("Neither systemd-inhibit nor xset found: %v", err)
			} else if string(output) == "" {
				t.Error("xset returned empty output")
			}
		}
	case "windows":
		// Windows testing would require checking system power state
		// This is more complex and might require admin privileges
		t.Log("Windows power state verification not implemented")
	}

	// Stop keep-alive
	if err := keeper.Stop(); err != nil {
		t.Errorf("Failed to stop keep-alive: %v", err)
	}

	// Give processes time to clean up
	time.Sleep(2 * time.Second)

	// Verify cleanup
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pgrep", "caffeinate")
		if err := cmd.Run(); err == nil {
			t.Error("caffeinate process still running after stop")
		}
	case "linux":
		cmd := exec.Command("pgrep", "-f", "systemd-inhibit")
		if err := cmd.Run(); err == nil {
			t.Error("systemd-inhibit process still running after stop")
		}
	}
}
