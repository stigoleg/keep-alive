package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stigoleg/keep-alive/v2/internal/power"
)

// TestPowerHoldVisibleInPmset checks that the power hold shows up as a
// PreventUserIdleSystemSleep assertion owned by our own caffeinate child and
// disappears on release. Other assertions on the machine are ignored.
func TestPowerHoldVisibleInPmset(t *testing.T) {
	if testing.Short() {
		t.Skip("holds a real power assertion")
	}
	hold, err := power.New().Acquire(context.Background(), power.Options{KeepDisplay: true, Reason: "e2e"})
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			hold.Release()
		}
	}()

	out, err := exec.Command("pgrep", "-P", strconv.Itoa(os.Getpid()), "caffeinate").Output()
	if err != nil {
		t.Fatalf("no caffeinate child: %v", err)
	}
	pid := strings.Fields(string(out))[0]
	owner := fmt.Sprintf("pid %s(caffeinate)", pid)

	if a := assertions(t); !strings.Contains(a, owner) || !strings.Contains(a, "PreventUserIdleSystemSleep") {
		t.Fatalf("pmset does not list %s:\n%s", owner, a)
	}
	if err := hold.Release(); err != nil {
		t.Fatal(err)
	}
	released = true
	if a := assertions(t); strings.Contains(a, owner) {
		t.Fatalf("assertion %s still listed after release", owner)
	}
}

func assertions(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("pmset", "-g", "assertions").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
