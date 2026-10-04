//go:build linux

package activity

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

type uinputOp struct {
	kind string // "ioctl", "ioctlBuf", "write", "close"
	req  uintptr
	arg  uintptr
	data []byte
}

type uinputRecorder struct {
	ops      []uinputOp
	setupErr error
}

func (r *uinputRecorder) ioctl(req, arg uintptr) error {
	r.ops = append(r.ops, uinputOp{kind: "ioctl", req: req, arg: arg})
	return nil
}

func (r *uinputRecorder) ioctlBuf(req uintptr, buf []byte) error {
	r.ops = append(r.ops, uinputOp{kind: "ioctlBuf", req: req, data: append([]byte(nil), buf...)})
	if req == uiDevSetup {
		return r.setupErr
	}
	return nil
}

func (r *uinputRecorder) write(b []byte) error {
	r.ops = append(r.ops, uinputOp{kind: "write", data: append([]byte(nil), b...)})
	return nil
}

func (r *uinputRecorder) close() error {
	r.ops = append(r.ops, uinputOp{kind: "close"})
	return nil
}

func newRecordedUinput(keys bool, rec *uinputRecorder) (*uinputInjector, *[]time.Duration, *int) {
	var slept []time.Duration
	opens := 0
	u := newUinput(keys)
	u.open = func(path string) (uinputFile, error) {
		if path != "/dev/uinput" {
			return nil, errors.New("wrong path " + path)
		}
		opens++
		return rec, nil
	}
	u.probe = func(string) error { return nil }
	u.sleep = func(d time.Duration) { slept = append(slept, d) }
	return u, &slept, &opens
}

func (r *uinputRecorder) bits(req uintptr) []uintptr {
	var out []uintptr
	for _, op := range r.ops {
		if op.kind == "ioctl" && op.req == req {
			out = append(out, op.arg)
		}
	}
	return out
}

func contains(xs []uintptr, x uintptr) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

type event struct {
	typ, code uint16
	value     int32
}

func decodeEvents(t *testing.T, b []byte) []event {
	t.Helper()
	word := int(unsafe.Sizeof(uintptr(0)))
	size := 2*word + 8
	if len(b)%size != 0 {
		t.Fatalf("write of %d bytes is not a whole number of %d-byte events", len(b), size)
	}
	var out []event
	for off := 0; off < len(b); off += size {
		e := b[off : off+size]
		out = append(out, event{
			typ:   binary.NativeEndian.Uint16(e[2*word:]),
			code:  binary.NativeEndian.Uint16(e[2*word+2:]),
			value: int32(binary.NativeEndian.Uint32(e[2*word+4:])),
		})
	}
	return out
}

func TestUinputCreatesAPointerLibinputAccepts(t *testing.T) {
	rec := &uinputRecorder{}
	u, slept, opens := newRecordedUinput(false, rec)
	if err := u.Available(); err != nil {
		t.Fatal(err)
	}
	if *opens != 0 {
		t.Fatal("device created before the first burst")
	}
	if err := u.MoveBy(3, -2); err != nil {
		t.Fatal(err)
	}

	ev := rec.bits(uiSetEvbit)
	for _, want := range []uintptr{evKey, evRel, evSyn} {
		if !contains(ev, want) {
			t.Fatalf("EV bits %v missing %#x", ev, want)
		}
	}
	keys := rec.bits(uiSetKeybit)
	for _, want := range []uintptr{btnLeft, btnRight, btnMiddle} {
		if !contains(keys, want) {
			t.Fatalf("key bits %v missing %#x (udev needs BTN_LEFT to tag a mouse)", keys, want)
		}
	}
	if contains(keys, keyRightShift) {
		t.Fatal("KEY_RIGHTSHIFT declared without --active-keys")
	}
	rel := rec.bits(uiSetRelbit)
	for _, want := range []uintptr{relX, relY, relWheel} {
		if !contains(rel, want) {
			t.Fatalf("rel bits %v missing %#x", rel, want)
		}
	}

	var setupAt, createAt, firstWrite = -1, -1, -1
	for i, op := range rec.ops {
		switch {
		case op.kind == "ioctlBuf" && op.req == uiDevSetup:
			setupAt = i
			if len(op.data) != 92 {
				t.Fatalf("uinput_setup is %d bytes, want 92", len(op.data))
			}
			if got := string(bytes.TrimRight(op.data[8:88], "\x00")); got != "keepalive virtual pointer" {
				t.Fatalf("device name %q", got)
			}
			if binary.NativeEndian.Uint16(op.data[0:]) != busUSB {
				t.Fatal("bus type is not USB")
			}
		case op.kind == "ioctl" && op.req == uiDevCreate:
			createAt = i
		case op.kind == "write" && firstWrite < 0:
			firstWrite = i
		}
	}
	if setupAt < 0 || createAt < setupAt || firstWrite < createAt {
		t.Fatalf("want capability ioctls, UI_DEV_SETUP, UI_DEV_CREATE, then events; got %+v", rec.ops)
	}
	for _, op := range rec.ops[:setupAt] {
		if op.kind != "ioctl" || (op.req != uiSetEvbit && op.req != uiSetKeybit && op.req != uiSetRelbit) {
			t.Fatalf("unexpected op before UI_DEV_SETUP: %+v", op)
		}
	}
	if len(*slept) != 1 || (*slept)[0] != 250*time.Millisecond {
		t.Fatalf("slept %v after UI_DEV_CREATE, want 250ms", *slept)
	}

	got := decodeEvents(t, rec.ops[firstWrite].data)
	want := []event{{evRel, relX, 3}, {evRel, relY, -2}, {evSyn, synReport, 0}}
	if len(got) != len(want) {
		t.Fatalf("events %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events %+v, want %+v", got, want)
		}
	}

	// Later moves reuse the device and never send a zero axis.
	if err := u.MoveBy(0, 5); err != nil {
		t.Fatal(err)
	}
	if *opens != 1 {
		t.Fatalf("device opened %d times", *opens)
	}
	last := decodeEvents(t, rec.ops[len(rec.ops)-1].data)
	if len(last) != 2 || last[0] != (event{evRel, relY, 5}) {
		t.Fatalf("events %+v, want REL_Y 5 + SYN only", last)
	}

	n := len(rec.ops)
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	if rec.ops[n].req != uiDevDestroy || rec.ops[n+1].kind != "close" {
		t.Fatalf("close ops %+v", rec.ops[n:])
	}
}

func TestUinputFallsBackToUserDevOnOldKernels(t *testing.T) {
	rec := &uinputRecorder{setupErr: syscall.EINVAL}
	u, _, _ := newRecordedUinput(true, rec)
	if err := u.MoveBy(1, 1); err != nil {
		t.Fatal(err)
	}
	if !contains(rec.bits(uiSetKeybit), keyRightShift) {
		t.Fatal("KEY_RIGHTSHIFT not declared with keys enabled")
	}
	var userDev []byte
	createAt := -1
	for i, op := range rec.ops {
		if op.kind == "write" && userDev == nil {
			userDev = op.data
		}
		if op.kind == "ioctl" && op.req == uiDevCreate {
			createAt = i
		}
	}
	if len(userDev) != 1116 {
		t.Fatalf("uinput_user_dev write is %d bytes, want 1116", len(userDev))
	}
	if got := string(bytes.TrimRight(userDev[:80], "\x00")); got != "keepalive virtual pointer" {
		t.Fatalf("device name %q", got)
	}
	if createAt < 0 {
		t.Fatal("UI_DEV_CREATE not called after the fallback")
	}

	if err := u.Tap(); err != nil {
		t.Fatal(err)
	}
	got := decodeEvents(t, rec.ops[len(rec.ops)-1].data)
	want := []event{{evKey, keyRightShift, 1}, {evSyn, synReport, 0}, {evKey, keyRightShift, 0}, {evSyn, synReport, 0}}
	for i := range want {
		if len(got) != len(want) || got[i] != want[i] {
			t.Fatalf("tap events %+v, want %+v", got, want)
		}
	}
}

func TestUinputOtherSetupErrorsFail(t *testing.T) {
	rec := &uinputRecorder{setupErr: syscall.EPERM}
	u, _, _ := newRecordedUinput(false, rec)
	if err := u.MoveBy(1, 1); err == nil {
		t.Fatal("MoveBy succeeded although UI_DEV_SETUP failed")
	}
	if rec.ops[len(rec.ops)-1].kind != "close" {
		t.Fatal("device not closed after a failed setup")
	}
}

func TestUinputAvailabilityHints(t *testing.T) {
	u := newUinput(false)
	u.probe = func(string) error { return syscall.ENOENT }
	var un *Unavailable
	if err := u.Available(); !errors.As(err, &un) || un.Hint == "" || !bytes.Contains([]byte(un.Hint), []byte("modprobe uinput")) {
		t.Fatalf("missing device: %v", err)
	}
	u.probe = func(string) error { return syscall.EACCES }
	if err := u.Available(); !errors.As(err, &un) || un.Hint != uinputPermissionHint {
		t.Fatalf("no permission: %v", err)
	}
}

func TestUinputPermissionHintUsesTheSeatNotTheInputGroup(t *testing.T) {
	h := uinputPermissionHint
	for _, want := range []string{
		`echo 'KERNEL=="uinput", SUBSYSTEM=="misc", TAG+="uaccess", OPTIONS+="static_node=uinput"' | sudo tee /etc/udev/rules.d/60-keepalive-uinput.rules`,
		"sudo udevadm control --reload && sudo udevadm trigger",
		"sudo modprobe -r uinput && sudo modprobe uinput",
		`"input" group`, // only as an alternative, with its caveat
		"read your keyboard",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("hint lacks %q:\n%s", want, h)
		}
	}
	if strings.Contains(h, "usermod") || strings.Contains(h, `GROUP="input"`) {
		t.Errorf("hint still tells the user to join the input group:\n%s", h)
	}
	// Every Linux hint that sends the user to uinput carries it.
	y := &ydotool{run: (&fakeRunner{reply: func(call) (string, string, error) { return ydotool018Help, "", nil }}).run, lookPath: found}
	var un *Unavailable
	if err := y.Available(); !errors.As(err, &un) || !strings.Contains(un.Hint, h) {
		t.Errorf("ydotool 0.1.x hint: %v", err)
	}
	x := &xdotool{lookPath: found, env: linuxEnv{display: ":0", wayland: "wayland-0"}}
	if err := x.Available(); !errors.As(err, &un) || !strings.Contains(un.Hint, h) {
		t.Errorf("xdotool on Wayland hint: %v", err)
	}
	if _, hint := x.Diagnose(); !strings.Contains(hint, h) {
		t.Errorf("xdotool diagnosis hint %q", hint)
	}
}
