//go:build !windows

package cli

import (
	"strings"
	"testing"

	"github.com/stigoleg/keep-alive/v2/internal/power"
)

// TestRunWithoutPowerStillRunsTheCommand: a missing power hold is a warning;
// the command runs and its exit code is keepalive's.
func TestRunWithoutPowerStillRunsTheCommand(t *testing.T) {
	ta := newTestApp(t)
	ta.runChild = nil // the real keepalive run
	ta.newPower = func() power.Inhibitor { return failingPower{} }
	ta.Notifier = &recordingNotifier{}
	if code := ta.run("run", "--", "sh", "-c", "echo ran; exit 3"); code != 3 {
		t.Fatalf("exit %d, want the command's 3 (stderr %q)", code, ta.stderr)
	}
	if ta.stdout.String() != "ran\n" {
		t.Fatalf("stdout = %q", ta.stdout)
	}
	errOut := ta.stderr.String()
	if !strings.HasPrefix(errOut, "keepalive: warning: could not keep the system awake: no inhibitor answered\nhint: start a desktop session") {
		t.Fatalf("stderr = %q", errOut)
	}
	if strings.Contains(errOut, "error") {
		t.Fatalf("stderr reports an error: %q", errOut)
	}
}
