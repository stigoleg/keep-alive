package power

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCaffeinateArgs(t *testing.T) {
	if got, want := caffeinateArgs(true, 42), []string{"-i", "-d", "-s", "-w", "42"}; !reflect.DeepEqual(got, want) {
		t.Errorf("caffeinateArgs(true) = %v, want %v", got, want)
	}
	if got, want := caffeinateArgs(false, 42), []string{"-i", "-s", "-w", "42"}; !reflect.DeepEqual(got, want) {
		t.Errorf("caffeinateArgs(false) = %v, want %v", got, want)
	}
}

func TestCaffeinateHoldVisibleInPmset(t *testing.T) {
	if testing.Short() {
		t.Skip("holds real power assertions")
	}
	for _, display := range []bool{true, false} {
		h, err := caffeinateInhibitor{}.Acquire(context.Background(), Options{KeepDisplay: display})
		if err != nil {
			t.Fatalf("Acquire(display=%v): %v", display, err)
		}
		p := h.(*procHold)
		pid := p.cmd.Process.Pid
		if !strings.Contains(h.Describe(), "-w "+strconv.Itoa(os.Getpid())) {
			t.Errorf("Describe() = %q, want -w <our pid>", h.Describe())
		}
		checkHoldTypes(t, pid, display)
		if err := h.Release(); err != nil {
			t.Fatalf("Release: %v", err)
		}
		if err := h.Release(); err != nil {
			t.Fatalf("second Release: %v", err)
		}
		if alive(pid) {
			t.Fatalf("caffeinate %d still running after Release", pid)
		}
		if got := waitAssertions(t, pid, 2*time.Second, isEmpty); len(got) != 0 {
			t.Fatalf("caffeinate %d assertions still listed: %v", pid, got)
		}
		select {
		case err := <-p.Lost():
			t.Fatalf("Lost fired after Release: %v", err)
		default:
		}
	}
}

func TestCaffeinateLostWhenKilled(t *testing.T) {
	if testing.Short() {
		t.Skip("runs caffeinate")
	}
	h, err := caffeinateInhibitor{}.Acquire(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	p := h.(*procHold)
	p.cmd.Process.Kill() // our own child
	select {
	case err := <-p.Lost():
		if !strings.Contains(err.Error(), "caffeinate") {
			t.Errorf("loss error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Lost did not fire when caffeinate died")
	}
}

func TestCaffeinateExitsWithKilledOwner(t *testing.T) {
	testKilledOwnerReleases(t, "caffeinate")
}

func TestCaffeinateMechanism(t *testing.T) {
	if m := caffeinateMechanism(); !m.Available {
		t.Fatalf("caffeinate unavailable: %+v", m)
	}
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
