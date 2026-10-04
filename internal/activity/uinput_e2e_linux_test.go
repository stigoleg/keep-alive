//go:build linux

package activity

import (
	"os"
	"strings"
	"testing"
)

// TestUinputDeviceOnRealKernel creates the virtual pointer for real and
// checks the capabilities the kernel reports, which are what udev's
// input_id builtin uses to tag ID_INPUT_MOUSE. It moves the pointer on a
// desktop, so it only runs with KEEPALIVE_E2E_UINPUT=1 (e.g. in a
// privileged container).
func TestUinputDeviceOnRealKernel(t *testing.T) {
	if os.Getenv("KEEPALIVE_E2E_UINPUT") != "1" {
		t.Skip("creates a real input device; set KEEPALIVE_E2E_UINPUT=1 to run")
	}
	u := newUinput(true)
	if err := u.Available(); err != nil {
		t.Fatal(err)
	}
	if err := u.MoveBy(2, -1); err != nil {
		t.Fatalf("MoveBy: %v", err)
	}
	if err := u.Tap(); err != nil {
		t.Fatalf("Tap: %v", err)
	}
	block := inputDeviceBlock(t, uinputName)
	if block == "" {
		t.Fatalf("%q not listed in /proc/bus/input/devices", uinputName)
	}
	t.Logf("\n%s", block)
	bits := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		if k, v, ok := strings.Cut(strings.TrimPrefix(line, "B: "), "="); ok && strings.HasPrefix(line, "B: ") {
			bits[k] = v
		}
	}
	// EV: SYN|KEY|REL = 0x7; REL: X|Y|WHEEL = 0x103.
	if bits["EV"] != "7" {
		t.Errorf("EV bits = %q, want 7", bits["EV"])
	}
	if bits["REL"] != "103" {
		t.Errorf("REL bits = %q, want 103", bits["REL"])
	}
	key := strings.Fields(bits["KEY"])
	// The last word holds bits 0-63 (KEY_RIGHTSHIFT = 54); the fifth from
	// the end holds 256-319 (BTN_LEFT..BTN_MIDDLE = 0x110-0x112).
	if len(key) < 5 || key[len(key)-1] != "40000000000000" || key[len(key)-5] != "70000" {
		t.Errorf("KEY bits = %q, want BTN_LEFT/RIGHT/MIDDLE and KEY_RIGHTSHIFT", bits["KEY"])
	}
	if err := u.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if inputDeviceBlock(t, uinputName) != "" {
		t.Fatal("device still listed after Close")
	}
}

func inputDeviceBlock(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range strings.Split(string(b), "\n\n") {
		if strings.Contains(block, `N: Name="`+name+`"`) {
			return block
		}
	}
	return ""
}
