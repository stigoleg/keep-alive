//go:build linux

package linux

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// uinput constants.
const (
	uinputDevicePath = "/dev/uinput"
	uinputBusTypeUSB = 0x03
	uinputVendorID   = 0x1234
	uinputProductID  = 0x5678
	uinputDeviceName = "keep-alive-mouse"

	// Linux input event types
	evSyn = 0x00
	evRel = 0x02
	relX  = 0x00
	relY  = 0x01

	// uinput ioctl commands
	uiSetEvbit   = 0x40045564 // _IOW('U', 100, int)
	uiSetRelbit  = 0x40045565 // _IOW('U', 101, int)
	uiDevCreate  = 0x5501     // _IO('U', 1)
	uiDevDestroy = 0x5502     // _IO('U', 2)
)

type uinputUserDev struct {
	name [80]byte
	id   struct {
		bustype uint16
		vendor  uint16
		product uint16
		version uint16
	}
	ffEffectsMax uint32
	absmax       [64]int32
	absmin       [64]int32
	absfuzz      [64]int32
	absflat      [64]int32
}

type inputEvent struct {
	time  syscall.Timeval
	etype uint16
	code  uint16
	value int32
}

// UinputSimulator provides native Linux mouse simulation using the uinput kernel interface.
type UinputSimulator struct {
	fd   uintptr
	file *os.File
}

// Setup initializes the uinput device.
func (u *UinputSimulator) Setup() error {
	f, err := os.OpenFile(uinputDevicePath, os.O_WRONLY|syscall.O_NONBLOCK, 0660)
	if err != nil {
		return fmt.Errorf("failed to open uinput device: %w", err)
	}
	u.file = f
	u.fd = f.Fd()

	if err := u.enableRelativeAxes(); err != nil {
		u.Close()
		return fmt.Errorf("failed to enable relative axes: %w", err)
	}

	if err := u.createDevice(); err != nil {
		u.Close()
		return fmt.Errorf("failed to create uinput device: %w", err)
	}

	return nil
}

func (u *UinputSimulator) enableRelativeAxes() error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetEvbit), uintptr(evRel)); errno != 0 {
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetRelbit), uintptr(relX)); errno != 0 {
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetRelbit), uintptr(relY)); errno != 0 {
		return errno
	}
	return nil
}

func (u *UinputSimulator) createDevice() error {
	var dev uinputUserDev
	copy(dev.name[:], uinputDeviceName)
	dev.id.bustype = uinputBusTypeUSB
	dev.id.vendor = uinputVendorID
	dev.id.product = uinputProductID

	if _, _, errno := syscall.Syscall(syscall.SYS_WRITE, u.fd, uintptr(unsafe.Pointer(&dev)), unsafe.Sizeof(dev)); errno != 0 {
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiDevCreate), 0); errno != 0 {
		return errno
	}
	return nil
}

// Move moves the mouse by the specified relative amounts.
func (u *UinputSimulator) Move(dx, dy int32) error {
	events := []inputEvent{
		{etype: evRel, code: relX, value: dx},
		{etype: evRel, code: relY, value: dy},
		{etype: evSyn, code: 0, value: 0},
	}
	for _, ev := range events {
		_, err := syscall.Write(int(u.fd), (*[unsafe.Sizeof(ev)]byte)(unsafe.Pointer(&ev))[:])
		if err != nil {
			return err
		}
	}
	return nil
}

// Close releases the uinput device.
func (u *UinputSimulator) Close() {
	if u.fd != 0 {
		syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiDevDestroy), 0)
	}
	if u.file != nil {
		u.file.Close()
		u.file = nil
	}
	u.fd = 0
}
