//go:build windows

package activity

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// A 32-bit marker works with both INPUT layouts. Only our tagged events are
// ignored; remote-control and accessibility input still count as user input.
const inputMarker = 0x4b414c56

var (
	procSetWindowsHookEx    = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procPeekMessage         = user32.NewProc("PeekMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	procCurrentThreadID     = kernel32.NewProc("GetCurrentThreadId")
	inputHooks              = struct {
		sync.RWMutex
		threads map[uintptr]*windowsMonitor
	}{threads: make(map[uintptr]*windowsMonitor)}
	mouseCallback    = syscall.NewCallback(observeMouse)
	keyboardCallback = syscall.NewCallback(observeKeyboard)
)

type mouseHookEvent struct {
	point     point32
	mouseData uint32
	flags     uint32
	time      uint32
	extra     uintptr
}

type keyboardHookEvent struct {
	key, scan, flags, time uint32
	extra                  uintptr
}

type inputMessage struct {
	window  uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   point32
	private uint32
}

type windowsMonitor struct {
	*inputState
	cancel   context.CancelFunc
	done     chan struct{}
	pulse    atomic.Int64
	observed atomic.Uint32
}

func (*windowsMonitor) Name() string { return "Windows keyboard and mouse hooks" }

func (m *windowsMonitor) Snapshot() inputSnapshot {
	if time.Since(time.Unix(0, m.pulse.Load())) > 100*time.Millisecond {
		m.fail(&Unavailable{Reason: "Windows input observer stopped responding"})
	}
	// Windows can silently remove a low-level hook after a timeout. An
	// unexplained advance of LASTINPUTINFO invalidates coverage rather than
	// allowing a stale observer to declare the user idle. Tagged synthetic
	// events update observed too, so they do not trip this check.
	lii := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii))); r == 0 {
		m.fail(&Unavailable{Reason: "cannot verify Windows input-monitor coverage"})
	} else if int32(lii.dwTime-m.observed.Load()) > 0 {
		m.fail(&Unavailable{Reason: "Windows input was not seen by the observer; monitoring must be refreshed"})
	}
	return m.inputState.Snapshot()
}

func (m *windowsMonitor) Close() error {
	m.cancel()
	<-m.done
	return nil
}

func hookMonitor() *windowsMonitor {
	id, _, _ := procCurrentThreadID.Call()
	inputHooks.RLock()
	m := inputHooks.threads[id]
	inputHooks.RUnlock()
	return m
}

func observeMouse(code, message uintptr, event *mouseHookEvent) uintptr {
	if int32(code) >= 0 {
		if m := hookMonitor(); m != nil {
			m.observed.Store(event.time)
			if event.extra != inputMarker {
				var button uint64
				down := false
				switch message {
				case 0x0201, 0x0202: // left down/up
					button, down = 1, message == 0x0201
				case 0x0204, 0x0205: // right down/up
					button, down = 2, message == 0x0204
				case 0x0207, 0x0208: // middle down/up
					button, down = 4, message == 0x0207
				case 0x020B, 0x020C: // X button down/up
					button, down = 4+uint64(event.mouseData>>16), message == 0x020B
				}
				m.input(button, down) // motion, horizontal and vertical wheel use 0
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, code, message, uintptr(unsafe.Pointer(event)))
	return r
}

func observeKeyboard(code, message uintptr, event *keyboardHookEvent) uintptr {
	if int32(code) >= 0 {
		if m := hookMonitor(); m != nil {
			m.observed.Store(event.time)
			if event.extra != inputMarker {
				m.input(0x100+uint64(event.key), event.flags&0x80 == 0)
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, code, message, uintptr(unsafe.Pointer(event)))
	return r
}

func openWindowsMonitor(parent context.Context) (inputMonitor, error) {
	ctx, cancel := context.WithCancel(parent)
	m := &windowsMonitor{inputState: newInputState(time.Now), cancel: cancel, done: make(chan struct{})}
	m.pulse.Store(time.Now().UnixNano())
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(m.done)
		var message inputMessage
		// Create this thread's queue before publishing its hook callbacks.
		procPeekMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 0)
		id, _, _ := procCurrentThreadID.Call()
		inputHooks.Lock()
		inputHooks.threads[id] = m
		inputHooks.Unlock()
		defer func() {
			inputHooks.Lock()
			delete(inputHooks.threads, id)
			inputHooks.Unlock()
		}()
		mouse, _, err := procSetWindowsHookEx.Call(14, mouseCallback, 0, 0) // WH_MOUSE_LL
		if mouse == 0 {
			ready <- &Unavailable{Reason: fmt.Sprintf("cannot monitor mouse input: %v", err), Hint: "run keepalive in the signed-in user's interactive session"}
			return
		}
		defer procUnhookWindowsHookEx.Call(mouse)
		keyboard, _, err := procSetWindowsHookEx.Call(13, keyboardCallback, 0, 0) // WH_KEYBOARD_LL
		if keyboard == 0 {
			ready <- &Unavailable{Reason: fmt.Sprintf("cannot monitor keyboard input: %v", err)}
			return
		}
		defer procUnhookWindowsHookEx.Call(keyboard)
		for key := uintptr(1); key < 256; key++ {
			if key == 3 || key == 7 { // VK_CANCEL and an undefined mouse VK
				continue
			}
			if state, _, _ := procGetAsyncKeyState.Call(key); state&0x8000 != 0 {
				code := uint64(key)
				if key > 6 {
					// Generic modifier VKs duplicate their left/right versions.
					if key == 0x10 || key == 0x11 || key == 0x12 {
						continue
					}
					code += 0x100
				}
				m.input(code, true)
			}
		}
		lii := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
		if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii))); r == 0 {
			ready <- &Unavailable{Reason: "cannot verify initial Windows input state"}
			return
		}
		m.observed.Store(lii.dwTime)
		m.pulse.Store(time.Now().UnixNano())
		ready <- nil
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for ctx.Err() == nil {
			for n := 0; n < 64; n++ {
				r, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 1)
				if r == 0 {
					break
				}
				if message.message == 0x0012 { // WM_QUIT
					m.fail(&Unavailable{Reason: "Windows input observer exited"})
					return
				}
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
				procDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
			}
			m.pulse.Store(time.Now().UnixNano())
			select {
			case <-ctx.Done():
			case <-ticker.C:
			}
		}
		m.fail(ctx.Err())
	}()
	if err := <-ready; err != nil {
		cancel()
		<-m.done
		return nil, err
	}
	return m, nil
}
