//go:build windows

package activity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewLazySystemDLL loads only from System32, so a DLL planted next to
// keepalive.exe or in the working directory is never picked up.
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	wtsapi32 = windows.NewLazySystemDLL("wtsapi32.dll")

	procSendInput                     = user32.NewProc("SendInput")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procSetCursorPos                  = user32.NewProc("SetCursorPos")
	procOpenInputDesktop              = user32.NewProc("OpenInputDesktop")
	procCloseDesktop                  = user32.NewProc("CloseDesktop")
	procGetUserObjectInformationW     = user32.NewProc("GetUserObjectInformationW")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procGetLastInputInfo              = user32.NewProc("GetLastInputInfo")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetTickCount                  = kernel32.NewProc("GetTickCount")
	procProcessIdToSessionId          = kernel32.NewProc("ProcessIdToSessionId")
	procWTSQuerySessionInformationW   = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory                 = wtsapi32.NewProc("WTSFreeMemory")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfMove        = 0x0001
	mouseeventfVirtualDesk = 0x4000
	mouseeventfAbsolute    = 0x8000
	keyeventfKeyUp         = 0x0002

	vkRShift   = 0xA1
	scanRShift = 0x36

	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	wtsCurrentSession     = 0xFFFFFFFF
	wtsSessionInfoEx      = 25
	wtsSessionStateLock   = 0
	wtsSessionStateUnlock = 1

	desktopSwitchDesktop = 0x0100
	uoiName              = 2

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is the handle value -4.
	dpiAwarenessPerMonitorV2 = ^uintptr(3)
)

// MOUSEINPUT and KEYBDINPUT inside INPUT. The keyboard variant is padded to
// the size of the union's largest member.
type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type mouseInputEvent struct {
	inputType uint32
	mi        mouseInput
}

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type keyInputEvent struct {
	inputType uint32
	ki        keybdInput
	_         [8]byte
}

// SendInput rejects any cbSize but sizeof(INPUT); these fail to compile if
// either layout drifts from wantInputSize (40 on 64-bit, 28 on 32-bit).
var (
	_ [unsafe.Sizeof(mouseInputEvent{}) - wantInputSize]byte
	_ [wantInputSize - unsafe.Sizeof(mouseInputEvent{})]byte
	_ [unsafe.Sizeof(keyInputEvent{}) - wantInputSize]byte
	_ [wantInputSize - unsafe.Sizeof(keyInputEvent{})]byte
)

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

type point32 struct{ x, y int32 }

// wtsInfoEx is the head of WTSINFOEXW: Level, then the level-1 union, which
// is 8-byte aligned because it holds LARGE_INTEGERs.
type wtsInfoEx struct {
	level        uint32
	_            uint32
	sessionID    uint32
	sessionState uint32
	sessionFlags int32
}

func newBackend(_ context.Context, keys bool) *backend {
	idle := lastInputIdle{}
	return &backend{
		idle:    idle,
		sources: []IdleSource{idle},
		lock:    wtsLock{},
		open:    openSendInput,

		candidates: func() []Injector { return []Injector{sendInput{}} },
		lockName:   "WTS session state, else the input desktop",
	}
}

// lastInputIdle is what Chromium reads: GetTickCount() - LASTINPUTINFO.dwTime.
// SendInput-injected input updates it.
type lastInputIdle struct{}

func (lastInputIdle) Name() string { return "GetLastInputInfo" }

func (lastInputIdle) Idle() (time.Duration, error) {
	lii := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	if r, _, err := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii))); r == 0 {
		return 0, fmt.Errorf("GetLastInputInfo: %w", err)
	}
	now, _, _ := procGetTickCount.Call()
	// uint32 subtraction survives the 49.7-day tick wrap.
	return time.Duration(uint32(now)-lii.dwTime) * time.Millisecond, nil
}

// wtsLock reads the session lock flag, and the input desktop when the flag
// is unavailable or unknown.
type wtsLock struct{}

func (wtsLock) Locked() (bool, error) {
	return lockedSessionOrDesktop(wtsSessionLocked, inputDesktopIsLocked)
}

// wtsSessionLocked reads the session lock flag. Windows 7 reports it
// inverted, but Go no longer runs there.
func wtsSessionLocked() (bool, error) {
	var info *wtsInfoEx
	var n uint32
	r, _, err := procWTSQuerySessionInformationW.Call(0, wtsCurrentSession, wtsSessionInfoEx,
		uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return false, fmt.Errorf("WTSQuerySessionInformation: %w", err)
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(info)))
	if info == nil || n < uint32(unsafe.Sizeof(wtsInfoEx{})) || info.level != 1 {
		return false, errors.New("unexpected WTSINFOEX reply")
	}
	switch info.sessionFlags {
	case wtsSessionStateLock:
		return true, nil
	case wtsSessionStateUnlock:
		return false, nil
	}
	return false, fmt.Errorf("unknown session lock state %d", info.sessionFlags)
}

// inputDesktopIsLocked opens the desktop that receives input: the user's
// "Default" one, or Winlogon's secure desktop while the session is locked.
func inputDesktopIsLocked() (bool, error) {
	h, _, err := procOpenInputDesktop.Call(0, 0, desktopSwitchDesktop)
	if h == 0 {
		return inputDesktopLocked("", fmt.Errorf("OpenInputDesktop: %w", err), errors.Is(err, windows.ERROR_ACCESS_DENIED))
	}
	defer procCloseDesktop.Call(h)
	var name [256]uint16
	var n uint32
	if r, _, err := procGetUserObjectInformationW.Call(h, uoiName, uintptr(unsafe.Pointer(&name[0])),
		uintptr(len(name)*2), uintptr(unsafe.Pointer(&n))); r == 0 {
		return false, fmt.Errorf("GetUserObjectInformation: %w", err)
	}
	return inputDesktopLocked(windows.UTF16ToString(name[:]), nil, false)
}

var dpiAware sync.Once

func openSendInput() (Injector, error) {
	inj := sendInput{}
	if err := inj.Available(); err != nil {
		return nil, err
	}
	// Without DPI awareness GetCursorPos and the virtual-screen metrics are
	// scaled per process, and the return to the origin can land a pixel off.
	dpiAware.Do(func() {
		if procSetProcessDpiAwarenessContext.Find() == nil {
			procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
		}
	})
	return inj, nil
}

// sendInput moves the pointer with absolute SendInput events, which are not
// subject to pointer acceleration.
type sendInput struct{}

func (sendInput) Name() string { return "SendInput" }

func (sendInput) Available() error {
	var sid uint32
	r, _, _ := procProcessIdToSessionId.Call(uintptr(syscall.Getpid()), uintptr(unsafe.Pointer(&sid)))
	if r != 0 && sid == 0 {
		return &Unavailable{
			Reason: "keepalive runs in session 0 (as a service), which has no interactive desktop",
			Hint:   `run keepalive in the signed-in user's session, e.g. a Task Scheduler task set to "Run only when user is logged on"`,
		}
	}
	return nil
}

func (sendInput) Position() (float64, float64, bool) {
	var p point32
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		return 0, 0, false
	}
	return float64(p.x), float64(p.y), true
}

func (sendInput) Bounds() (Rect, bool) {
	r := virtualScreen()
	return r, r.W > 0 && r.H > 0
}

// FinishAt corrects a return stroke that landed a pixel or two off.
func (s sendInput) FinishAt(ox, oy float64) {
	set := func(x, y int32) error {
		if r, _, err := procSetCursorPos.Call(uintptr(x), uintptr(y)); r == 0 {
			return fmt.Errorf("SetCursorPos: %w", err)
		}
		return nil
	}
	if _, err := snapBack(s.Position, set, ox, oy); err != nil {
		slog.Debug("activity: cursor not put back exactly", "err", err)
	}
}

func virtualScreen() Rect {
	metric := func(i uintptr) float64 {
		v, _, _ := procGetSystemMetrics.Call(i)
		return float64(int32(v))
	}
	return Rect{X: metric(smXVirtualScreen), Y: metric(smYVirtualScreen), W: metric(smCXVirtualScreen), H: metric(smCYVirtualScreen)}
}

func (sendInput) MoveTo(x, y float64) error {
	nx, ny := virtualDeskAbsolute(x, y, virtualScreen())
	in := []mouseInputEvent{{inputType: inputMouse, mi: mouseInput{
		dx: nx, dy: ny, dwFlags: mouseeventfMove | mouseeventfAbsolute | mouseeventfVirtualDesk,
	}}}
	return send(len(in), unsafe.Pointer(&in[0]))
}

func (sendInput) Tap() error {
	in := []keyInputEvent{
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkRShift, wScan: scanRShift}},
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkRShift, wScan: scanRShift, dwFlags: keyeventfKeyUp}},
	}
	return send(len(in), unsafe.Pointer(&in[0]))
}

func send(n int, p unsafe.Pointer) error {
	r, _, err := procSendInput.Call(uintptr(n), uintptr(p), wantInputSize)
	if int(r) != n {
		return fmt.Errorf("SendInput inserted %d of %d events: %w", r, n, err)
	}
	return nil
}

// Diagnose: SendInput reports success even when User Interface Privilege
// Isolation drops the input, so an ineffective burst usually means an
// elevated window has focus.
func (sendInput) Diagnose() (string, string) {
	return "Windows dropped the synthetic input",
		"input is blocked while an elevated (administrator) window is focused; run keepalive elevated or focus another window"
}

func (sendInput) Close() error { return nil }
