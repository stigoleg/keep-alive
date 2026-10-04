//go:build linux

package activity

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	uinputPath = "/dev/uinput"
	uinputName = "keepalive virtual pointer"
	// uinputSettle gives udev and libinput time to pick up a new device.
	uinputSettle = 250 * time.Millisecond

	busUSB         = 0x03
	uinputVendor   = 0x1234
	uinputProduct  = 0x5678
	uinputMaxName  = 80
	uinputAbsCount = 64

	evSyn = 0x00
	evKey = 0x01
	evRel = 0x02

	synReport = 0

	relX     = 0x00
	relY     = 0x01
	relWheel = 0x08

	btnLeft       = 0x110
	btnRight      = 0x111
	btnMiddle     = 0x112
	keyRightShift = 54

	// ioctl numbers, identical on amd64 and arm64.
	uiDevCreate  = 0x5501     // _IO('U', 1)
	uiDevDestroy = 0x5502     // _IO('U', 2)
	uiDevSetup   = 0x405c5503 // _IOW('U', 3, struct uinput_setup)
	uiSetEvbit   = 0x40045564 // _IOW('U', 100, int)
	uiSetKeybit  = 0x40045565 // _IOW('U', 101, int)
	uiSetRelbit  = 0x40045566 // _IOW('U', 102, int)
)

// uinputFile is the device handle, behind an interface so tests can record
// the exact ioctl sequence.
type uinputFile interface {
	ioctl(req, arg uintptr) error
	ioctlBuf(req uintptr, buf []byte) error
	write(b []byte) error
	close() error
}

type sysUinput struct{ fd int }

func openUinputDevice(path string) (uinputFile, error) {
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return sysUinput{fd: fd}, nil
}

func (s sysUinput) ioctl(req, arg uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(s.fd), req, arg); e != 0 {
		return e
	}
	return nil
}

func (s sysUinput) ioctlBuf(req uintptr, buf []byte) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(s.fd), req, uintptr(unsafe.Pointer(&buf[0]))); e != 0 {
		return e
	}
	return nil
}

func (s sysUinput) write(b []byte) error {
	n, err := syscall.Write(s.fd, b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return fmt.Errorf("short uinput write: %d of %d bytes", n, len(b))
	}
	return nil
}

func (s sysUinput) close() error { return syscall.Close(s.fd) }

// uinputInjector is a virtual relative pointer. udev tags it ID_INPUT_MOUSE
// (it has BTN_LEFT plus REL_X/REL_Y), so libinput and every compositor treat
// its events as real input.
type uinputInjector struct {
	keys   bool
	open   func(path string) (uinputFile, error)
	access func(path string) error
	sleep  func(time.Duration)

	mu  sync.Mutex
	dev uinputFile // created on the first burst
}

func newUinput(keys bool) *uinputInjector {
	return &uinputInjector{
		keys:   keys,
		open:   openUinputDevice,
		access: func(p string) error { return syscall.Access(p, 2 /* W_OK */) },
		sleep:  time.Sleep,
	}
}

func (u *uinputInjector) Name() string { return "uinput" }

func (u *uinputInjector) Available() error {
	u.mu.Lock()
	created := u.dev != nil
	u.mu.Unlock()
	if created {
		return nil
	}
	err := u.access(uinputPath)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, os.ErrNotExist):
		return &Unavailable{
			Reason: uinputPath + " does not exist",
			Hint:   "load the uinput module: sudo modprobe uinput (add uinput to /etc/modules-load.d/uinput.conf to keep it)",
		}
	default:
		return &Unavailable{Reason: "no write access to " + uinputPath, Hint: uinputPermissionHint}
	}
}

// device creates the virtual pointer on first use.
func (u *uinputInjector) device() (uinputFile, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.dev != nil {
		return u.dev, nil
	}
	f, err := u.open(uinputPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", uinputPath, err)
	}
	if err := u.setup(f); err != nil {
		_ = f.close()
		return nil, err
	}
	u.sleep(uinputSettle)
	u.dev = f
	return f, nil
}

func (u *uinputInjector) setup(f uinputFile) error {
	keys := []uintptr{btnLeft, btnRight, btnMiddle}
	if u.keys {
		keys = append(keys, keyRightShift)
	}
	steps := []struct {
		req  uintptr
		args []uintptr
	}{
		{uiSetEvbit, []uintptr{evKey, evRel, evSyn}},
		{uiSetKeybit, keys},
		{uiSetRelbit, []uintptr{relX, relY, relWheel}},
	}
	for _, s := range steps {
		for _, a := range s.args {
			if err := f.ioctl(s.req, a); err != nil {
				return fmt.Errorf("uinput ioctl %#x(%d): %w", s.req, a, err)
			}
		}
	}
	if err := f.ioctlBuf(uiDevSetup, uinputSetup()); err != nil {
		// Kernels before 4.5 lack UI_DEV_SETUP; they take uinput_user_dev
		// through write instead.
		if !errors.Is(err, syscall.EINVAL) {
			return fmt.Errorf("UI_DEV_SETUP: %w", err)
		}
		if err := f.write(uinputUserDev()); err != nil {
			return fmt.Errorf("write uinput_user_dev: %w", err)
		}
	}
	if err := f.ioctl(uiDevCreate, 0); err != nil {
		return fmt.Errorf("UI_DEV_CREATE: %w", err)
	}
	return nil
}

func (u *uinputInjector) MoveBy(dx, dy int) error {
	// The kernel drops EV_REL events with value 0.
	var b []byte
	if dx != 0 {
		b = append(b, inputEvent(evRel, relX, int32(dx))...)
	}
	if dy != 0 {
		b = append(b, inputEvent(evRel, relY, int32(dy))...)
	}
	if len(b) == 0 {
		return nil
	}
	return u.emit(append(b, inputEvent(evSyn, synReport, 0)...))
}

func (u *uinputInjector) Tap() error {
	if !u.keys {
		return errors.New("the virtual device was created without keys")
	}
	var b []byte
	b = append(b, inputEvent(evKey, keyRightShift, 1)...)
	b = append(b, inputEvent(evSyn, synReport, 0)...)
	b = append(b, inputEvent(evKey, keyRightShift, 0)...)
	b = append(b, inputEvent(evSyn, synReport, 0)...)
	return u.emit(b)
}

func (u *uinputInjector) emit(b []byte) error {
	dev, err := u.device()
	if err != nil {
		return err
	}
	return dev.write(b)
}

func (u *uinputInjector) Diagnose() (string, string) {
	if err := u.Available(); err != nil {
		var un *Unavailable
		if errors.As(err, &un) {
			return un.Reason, un.Hint
		}
	}
	return "the desktop ignored the virtual pointer",
		`check that "libinput list-devices" shows "` + uinputName + `" while keepalive runs`
}

func (u *uinputInjector) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.dev == nil {
		return nil
	}
	err := u.dev.ioctl(uiDevDestroy, 0)
	if cerr := u.dev.close(); err == nil {
		err = cerr
	}
	u.dev = nil
	return err
}

// inputEvent encodes struct input_event: a zero timeval (the kernel stamps
// it) of two machine words, then type, code and value.
func inputEvent(typ, code uint16, value int32) []byte {
	word := int(unsafe.Sizeof(uintptr(0)))
	b := make([]byte, 2*word+8)
	binary.NativeEndian.PutUint16(b[2*word:], typ)
	binary.NativeEndian.PutUint16(b[2*word+2:], code)
	binary.NativeEndian.PutUint32(b[2*word+4:], uint32(value))
	return b
}

func putInputID(b []byte) {
	binary.NativeEndian.PutUint16(b[0:], busUSB)
	binary.NativeEndian.PutUint16(b[2:], uinputVendor)
	binary.NativeEndian.PutUint16(b[4:], uinputProduct)
	binary.NativeEndian.PutUint16(b[6:], 1)
}

// uinputSetup encodes struct uinput_setup { input_id id; char name[80];
// u32 ff_effects_max; } (92 bytes).
func uinputSetup() []byte {
	b := make([]byte, 8+uinputMaxName+4)
	putInputID(b)
	copy(b[8:8+uinputMaxName-1], uinputName)
	return b
}

// uinputUserDev encodes the legacy struct uinput_user_dev { char name[80];
// input_id id; u32 ff_effects_max; s32 absmax, absmin, absfuzz,
// absflat[64]; } (1116 bytes).
func uinputUserDev() []byte {
	b := make([]byte, uinputMaxName+8+4+4*4*uinputAbsCount)
	copy(b[:uinputMaxName-1], uinputName)
	putInputID(b[uinputMaxName:])
	return b
}
