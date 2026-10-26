//go:build linux

package activity

import (
	"context"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const xInputTestDevices = `⎡ Virtual core pointer                     id=2 [master pointer (3)]
⎜   ↳ Virtual core XTEST pointer            id=4 [slave pointer (2)]
⎜   ↳ USB Mouse                             id=8 [slave pointer (2)]
⎜   ↳ keepalive virtual pointer              id=9 [slave pointer (2)]
⎜   ↳ ydotoold virtual device                id=10 [slave pointer (2)]
⎣ Virtual core keyboard                    id=3 [master keyboard (2)]
    ↳ Virtual core XTEST keyboard           id=5 [slave keyboard (3)]
    ↳ USB Keyboard                          id=11 [slave keyboard (3)]
`

func TestXInputDistinguishesRealInputFromSimulation(t *testing.T) {
	devices, err := parseXInputDevices(xInputTestDevices)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []int{4, 5, 9, 10} {
		if !devices[source] {
			t.Fatalf("source %d not excluded: %+v", source, devices[source])
		}
	}
	state := newInputState(time.Now)
	p := xInputParser{state: state, devices: devices}
	for _, line := range []string{"EVENT type 17 (RawMotion)", "device: 2 (4)", "detail: 0"} {
		if err := p.line(line); err != nil {
			t.Fatal(err)
		}
	}
	if state.Snapshot().Generation != 0 {
		t.Fatal("own XTEST movement counted as the user")
	}
	for _, line := range []string{"EVENT type 13 (RawKeyPress)", "device: 3 (11)", "detail: 38"} {
		if err := p.line(line); err != nil {
			t.Fatal(err)
		}
	}
	if !state.Snapshot().Held || state.Snapshot().Generation != 1 {
		t.Fatal("keyboard input missed")
	}
	for _, line := range []string{"EVENT type 14 (RawKeyRelease)", "device: 3 (11)", "detail: 38"} {
		if err := p.line(line); err != nil {
			t.Fatal(err)
		}
	}
	if state.Snapshot().Held {
		t.Fatal("key stayed held after release")
	}
	for _, line := range []string{"EVENT type 15 (RawButtonPress)", "device: 2 (8)", "detail: 4"} {
		if err := p.line(line); err != nil {
			t.Fatal(err)
		}
	}
	if state.Snapshot().Held || state.Snapshot().Generation != 3 {
		t.Fatal("scroll missed or mistaken for held input")
	}
}

func TestXInputFailsClosedForUnknownOrIncompleteEvents(t *testing.T) {
	devices, _ := parseXInputDevices(xInputTestDevices)
	for _, lines := range [][]string{
		{"EVENT type 17 (RawMotion)", "device: 2 (999)"},
		{"EVENT type 13 (RawKeyPress)", "EVENT type 17 (RawMotion)"},
		{"EVENT type 13 (RawKeyPress)", "detail: 38"},
		{"EVENT type 13 (RawKeyPress)", "device: invalid"},
		{"EVENT type 11 (HierarchyChanged)"},
	} {
		p := xInputParser{state: newInputState(time.Now), devices: devices}
		var err error
		for _, line := range lines {
			if err = p.line(line); err != nil {
				break
			}
		}
		if err == nil {
			t.Fatalf("accepted unobservable input: %v", lines)
		}
	}
	if _, err := parseXInputDevices("bad output"); err == nil {
		t.Fatal("invalid device list accepted")
	}
	if _, err := parseXInputDevices(xInputTestDevices + "extra tablet id=15 [floating slave]\n"); err == nil {
		t.Fatal("unobserved floating device accepted")
	}
	if syntheticDevice("remote desktop virtual keyboard") {
		t.Fatal("unrelated virtual input excluded")
	}
}

func TestEvdevTracksHeldInputAndRejectsDroppedEvents(t *testing.T) {
	state := newInputState(time.Now)
	for _, event := range [][]byte{inputEvent(evKey, 30, 1), inputEvent(evRel, 0, 1)} {
		if err := decodeEvdev(state, 2, event); err != nil {
			t.Fatal(err)
		}
	}
	if snap := state.Snapshot(); snap.Generation != 2 || !snap.Held {
		t.Fatalf("events missed: %+v", snap)
	}
	if err := decodeEvdev(state, 2, inputEvent(evKey, 30, 0)); err != nil {
		t.Fatal(err)
	}
	if state.Snapshot().Held {
		t.Fatal("key release missed")
	}
	if err := decodeEvdev(state, 2, inputEvent(evSyn, 3, 0)); err == nil {
		t.Fatal("dropped events did not invalidate monitoring")
	}
	if err := decodeEvdev(state, 2, []byte{1}); err == nil {
		t.Fatal("truncated events accepted")
	}
}

func TestEvdevRequiresCompleteDeviceEnumeration(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("sys/event0/device/name", "USB Mouse")
	write("sys/event0/device/capabilities/key", "0")
	write("sys/event0/device/capabilities/rel", "3")
	write("sys/event0/device/capabilities/abs", "0")
	write("sys/event0/dev", "13:64")
	write("sys/event1/device/name", uinputName)
	devices, err := evdevDevices(filepath.Join(root, "sys"), "/dev/input", filepath.Join(root, "udev"), "seat0")
	if err != nil || len(devices) != 1 || devices[0].name != "USB Mouse" {
		t.Fatalf("devices=%v error=%v", devices, err)
	}
	write("sys/event2/device/name", "USB Keyboard") // missing capabilities: do not silently skip it
	if _, err := evdevDevices(filepath.Join(root, "sys"), "/dev/input", filepath.Join(root, "udev"), "seat0"); err == nil {
		t.Fatal("partial device coverage accepted")
	}
}

func TestKernelCapabilityWords(t *testing.T) {
	for _, wordBits := range []int{32, 64} {
		bits, err := capabilityBits("1 0", wordBits)
		if err != nil || bits.Bit(wordBits) == 0 || bits.BitLen() != wordBits+1 {
			t.Fatalf("%d-bit bitmap=%v err=%v", wordBits, bits, err)
		}
	}
	if _, err := capabilityBits("not hex", 64); err == nil {
		t.Fatal("invalid bitmap accepted")
	}
	keys, axes := new(big.Int), new(big.Int)
	keys.SetBit(keys, 30, 1)
	if !relevantEvdev(keys, axes, axes) {
		t.Fatal("keyboard not considered relevant")
	}
}

func TestXdotoolUserCancellationDoesNotRestore(t *testing.T) {
	state := newInputState(time.Now)
	ctx, stop := watchInput(context.Background(), fakeMonitor{state}, 0)
	defer stop()
	started, done := make(chan struct{}), make(chan error, 1)
	restored := false
	x := &xdotool{run: func(ctx context.Context, _ time.Duration, _ []string, _ string, args ...string) (string, string, error) {
		if args[0] == "mousemove" {
			restored = true
			return "", "", nil
		}
		close(started)
		<-ctx.Done()
		return "", "", ctx.Err()
	}}
	go func() { done <- x.Play(ctx, 100, 100, NewPath(seeded(1))) }()
	<-started
	state.input(30, true)
	select {
	case err := <-done:
		if !errors.Is(err, errUserMoved) || restored {
			t.Fatalf("err=%v restored=%v", err, restored)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("chained helper did not yield")
	}
}

func TestXInputMissingHelperAndWaylandDeniedAccess(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := openXInputMonitor(context.Background()); err == nil {
		t.Fatal("missing xinput accepted")
	}
	// An unprivileged test with no input devices must fail closed. Containers
	// with observable devices exercise success in the opt-in integration test.
	if _, err := os.Stat("/sys/class/input"); os.IsNotExist(err) {
		if monitor, err := openEvdevMonitor(context.Background()); err == nil {
			monitor.Close()
			t.Fatal("missing input devices accepted")
		}
	}
	if !strings.Contains(evdevHint, "paused") {
		t.Fatal("missing recovery explanation")
	}
}

func TestWaylandDeniedReadAccessPausesMonitoring(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission denial requires an unprivileged process")
	}
	path := filepath.Join(t.TempDir(), "keyboard")
	if err := os.WriteFile(path, nil, 0o000); err != nil {
		t.Fatal(err)
	}
	monitor, err := openEvdevMonitorDevices(context.Background(), func() ([]evdevDevice, error) { return []evdevDevice{{path: path, name: "keyboard"}}, nil })
	if monitor != nil {
		monitor.Close()
		t.Fatal("monitor started without keyboard read access")
	}
	var unavailable *Unavailable
	if !errors.As(err, &unavailable) || unavailable.Hint != evdevHint {
		t.Fatalf("missing permission explanation: %v", err)
	}
}
