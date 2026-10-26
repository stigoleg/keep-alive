//go:build linux

package activity

import (
	"context"
	"encoding/binary"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const evdevHint = "activity is paused because complete input-device access is unavailable; use an X11 session with xinput, or a session that already grants access to its keyboard and pointer devices"

type evdevDevice struct {
	path string
	name string
}

type evdevMonitor struct {
	*inputState
	cancel context.CancelFunc
	done   chan struct{}
}

func (*evdevMonitor) Name() string { return "evdev user input" }
func (m *evdevMonitor) Close() error {
	m.cancel()
	<-m.done
	return nil
}

// sysfs capabilities are readable without opening a keyboard. Kernel bitmap
// words are printed most significant first, with spaces between native words.
func capabilityBits(text string, wordBits int) (*big.Int, error) {
	out := new(big.Int)
	for _, word := range strings.Fields(text) {
		n, ok := new(big.Int).SetString(word, 16)
		if !ok || n.BitLen() > wordBits {
			return nil, errors.New("invalid input-device capabilities")
		}
		out.Lsh(out, uint(wordBits))
		out.Or(out, n)
	}
	return out, nil
}

func relevantEvdev(key, rel, abs *big.Int) bool {
	// Keyboard keys, pointer buttons, relative axes and touch/pen axes.
	for code := 1; code < 0x100; code++ {
		if key.Bit(code) != 0 {
			return true
		}
	}
	for _, code := range []int{0x110, 0x111, 0x112, 0x140, 0x14a} {
		if key.Bit(code) != 0 {
			return true
		}
	}
	for _, code := range []int{0, 1, 6, 8, 11, 12} {
		if rel.Bit(code) != 0 {
			return true
		}
	}
	for _, code := range []int{0, 1, 0x35, 0x36} {
		if abs.Bit(code) != 0 {
			return true
		}
	}
	return false
}

func evdevDevices(sysRoot, devRoot, udevRoot, seat string) ([]evdevDevice, error) {
	entries, err := os.ReadDir(sysRoot)
	if err != nil {
		return nil, err
	}
	var devices []evdevDevice
	wordBits := 32
	if kernel64Bit() {
		wordBits = 64
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "event") {
			continue
		}
		root := filepath.Join(sysRoot, entry.Name())
		name, err := os.ReadFile(filepath.Join(root, "device/name"))
		if err != nil {
			return nil, errors.New("cannot identify an input device")
		}
		if syntheticDevice(strings.TrimSpace(string(name))) {
			continue
		}
		var caps [3]*big.Int
		for i, kind := range []string{"key", "rel", "abs"} {
			text, err := os.ReadFile(filepath.Join(root, "device/capabilities", kind))
			if err != nil {
				return nil, errors.New("cannot inspect input-device coverage")
			}
			// sysfs uses the kernel's word size, including for a 32-bit
			// executable running on a 64-bit kernel.
			caps[i], err = capabilityBits(string(text), wordBits)
			if err != nil {
				return nil, err
			}
		}
		if !relevantEvdev(caps[0], caps[1], caps[2]) {
			continue
		}
		dev, err := os.ReadFile(filepath.Join(root, "dev"))
		if err != nil {
			return nil, errors.New("cannot identify input-device seat")
		}
		metadata, err := os.ReadFile(filepath.Join(udevRoot, "c"+strings.TrimSpace(string(dev))))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		deviceSeat := "seat0"
		for _, line := range strings.Split(string(metadata), "\n") {
			if strings.HasPrefix(line, "E:ID_SEAT=") {
				deviceSeat = strings.TrimPrefix(line, "E:ID_SEAT=")
			}
		}
		if deviceSeat != seat {
			continue
		}
		devices = append(devices, evdevDevice{filepath.Join(devRoot, entry.Name()), strings.TrimSpace(string(name))})
	}
	if len(devices) == 0 {
		return nil, errors.New("no session keyboard or pointer devices are observable")
	}
	return devices, nil
}

func kernel64Bit() bool {
	var name unix.Utsname
	if unix.Uname(&name) != nil {
		return unsafe.Sizeof(uintptr(0)) == 8
	}
	var machine strings.Builder
	for _, c := range name.Machine {
		if c == 0 {
			break
		}
		machine.WriteByte(byte(c))
	}
	s := machine.String()
	return strings.Contains(s, "64") || s == "s390x"
}

func evdevHeld(fd int) ([]byte, error) {
	bits := make([]byte, 96) // KEY_MAX (0x2ff), including pointer buttons
	_, _, err := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(0x80004518)|uintptr(len(bits))<<16, uintptr(unsafe.Pointer(&bits[0])))
	if err != 0 {
		return nil, err
	}
	return bits, nil
}

func openEvdevMonitor(parent context.Context) (inputMonitor, error) {
	seat := os.Getenv("XDG_SEAT")
	if seat == "" {
		seat = "seat0"
	}
	list := func() ([]evdevDevice, error) {
		return evdevDevices("/sys/class/input", "/dev/input", "/run/udev/data", seat)
	}
	return openEvdevMonitorDevices(parent, list)
}

func openEvdevMonitorDevices(parent context.Context, list func() ([]evdevDevice, error)) (inputMonitor, error) {
	devices, err := list()
	if err != nil {
		return nil, &Unavailable{Reason: "cannot establish complete input-device coverage", Hint: evdevHint}
	}
	fds := make([]unix.PollFd, 0, len(devices))
	closeFDs := func() {
		for _, fd := range fds {
			_ = unix.Close(int(fd.Fd))
		}
	}
	state := newInputState(time.Now)
	for i, device := range devices {
		fd, err := unix.Open(device.path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			closeFDs()
			return nil, &Unavailable{Reason: "cannot monitor every session keyboard and pointer device", Hint: evdevHint}
		}
		fds = append(fds, unix.PollFd{Fd: int32(fd), Events: unix.POLLIN})
		held, err := evdevHeld(fd)
		if err != nil {
			closeFDs()
			return nil, &Unavailable{Reason: "cannot read initial input-device state", Hint: evdevHint}
		}
		for code := 0; code < len(held)*8; code++ {
			if held[code/8]&(1<<uint(code%8)) != 0 {
				state.input(uint64(i+1)<<32|uint64(code+1), true)
			}
		}
	}
	ctx, cancel := context.WithCancel(parent)
	m := &evdevMonitor{inputState: state, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(m.done)
		defer closeFDs()
		defer m.fail(&Unavailable{Reason: "input-device monitoring stopped", Hint: evdevHint})
		nextScan := time.Now().Add(50 * time.Millisecond)
		buffer := make([]byte, 4096)
		for ctx.Err() == nil {
			// A new unreadable device invalidates coverage before another
			// idle period is accepted. Reopening also refreshes held state.
			if !time.Now().Before(nextScan) {
				current, err := list()
				if err != nil || !sameEvdevDevices(devices, current) {
					m.fail(&Unavailable{Reason: "input devices changed; complete coverage must be refreshed", Hint: evdevHint})
					return
				}
				nextScan = time.Now().Add(50 * time.Millisecond)
			}
			if _, err := unix.Poll(fds, 10); err != nil {
				if errors.Is(err, unix.EINTR) {
					continue
				}
				return
			}
			for i, fd := range fds {
				if fd.Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
					return
				}
				if fd.Revents&unix.POLLIN == 0 {
					continue
				}
				n, err := unix.Read(int(fd.Fd), buffer)
				if errors.Is(err, unix.EAGAIN) {
					continue
				}
				if err != nil || n == 0 {
					return
				}
				if err := decodeEvdev(state, uint64(i+1), buffer[:n]); err != nil {
					m.fail(err)
					return
				}
			}
		}
	}()
	return m, nil
}

func sameEvdevDevices(a, b []evdevDevice) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func decodeEvdev(state *inputState, source uint64, data []byte) error {
	// input_event's timeval follows the executable's ABI (also on compat
	// kernels); inputEvent uses the same layout for our uinput writes.
	offset := 2 * int(unsafe.Sizeof(uintptr(0)))
	size := offset + 8
	if len(data)%size != 0 {
		return errors.New("invalid input-device event boundary")
	}
	for len(data) > 0 {
		typ := binary.NativeEndian.Uint16(data[offset:])
		code := binary.NativeEndian.Uint16(data[offset+2:])
		value := int32(binary.NativeEndian.Uint32(data[offset+4:]))
		switch typ {
		case evSyn:
			if code == 3 {
				return &Unavailable{Reason: "input events were dropped; monitoring must be refreshed", Hint: evdevHint}
			}
		case evKey:
			state.input(source<<32|uint64(code)+1, value != 0)
		case evRel:
			if value != 0 {
				state.input(0, false)
			}
		case 3: // EV_ABS (touch, pen, or absolute pointer)
			state.input(0, false)
		}
		data = data[size:]
	}
	return nil
}
