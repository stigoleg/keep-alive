//go:build darwin && cgo

package activity

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/hidsystem/IOLLEvent.h>
#include <stdlib.h>
#include <unistd.h>

#define KA_INPUT_MARKER 0x4b414c56

typedef struct {
	CFMachPortRef tap;
	CFRunLoopSourceRef source;
	unsigned long long generation;
	unsigned char keys[256];
	unsigned char buttons[32];
	int healthy;
} ka_monitor;

static CGEventMask ka_monitor_mask(void) {
	return CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventKeyUp) |
		CGEventMaskBit(kCGEventFlagsChanged) | CGEventMaskBit(kCGEventMouseMoved) |
		CGEventMaskBit(kCGEventLeftMouseDown) | CGEventMaskBit(kCGEventLeftMouseUp) |
		CGEventMaskBit(kCGEventRightMouseDown) | CGEventMaskBit(kCGEventRightMouseUp) |
		CGEventMaskBit(kCGEventOtherMouseDown) | CGEventMaskBit(kCGEventOtherMouseUp) |
		CGEventMaskBit(kCGEventLeftMouseDragged) | CGEventMaskBit(kCGEventRightMouseDragged) |
		CGEventMaskBit(kCGEventOtherMouseDragged) | CGEventMaskBit(kCGEventScrollWheel);
}

// The event flags describe the new state at the tap. Querying global key
// state here can still return the state before this flags-changed event.
static CGEventFlags ka_modifier_mask(int key) {
	switch (key) {
		case 0x36: return NX_DEVICERCMDKEYMASK;
		case 0x37: return NX_DEVICELCMDKEYMASK;
		case 0x38: return NX_DEVICELSHIFTKEYMASK;
		case 0x3a: return NX_DEVICELALTKEYMASK;
		case 0x3b: return NX_DEVICELCTLKEYMASK;
		case 0x3c: return NX_DEVICERSHIFTKEYMASK;
		case 0x3d: return NX_DEVICERALTKEYMASK;
		case 0x3e: return NX_DEVICERCTLKEYMASK;
		case 0x3f: return kCGEventFlagMaskSecondaryFn;
		default: return 0;
	}
}

static CGEventRef ka_observe(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *info) {
	ka_monitor *m = info;
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		m->healthy = 0;
		return event;
	}
	if (CGEventGetIntegerValueField(event, kCGEventSourceUserData) == KA_INPUT_MARKER &&
		CGEventGetIntegerValueField(event, kCGEventSourceUnixProcessID) == getpid()) {
		return event;
	}
	m->generation++;
	int key = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
	if (key >= 0 && key < 256) {
		if (type == kCGEventKeyDown) m->keys[key] = 1;
		if (type == kCGEventKeyUp) m->keys[key] = 0;
		if (type == kCGEventFlagsChanged && key != 0x39)
			m->keys[key] = (CGEventGetFlags(event) & ka_modifier_mask(key)) != 0;
	}
	int button = (int)CGEventGetIntegerValueField(event, kCGMouseEventButtonNumber);
	if (button >= 0 && button < 32) {
		if (type == kCGEventLeftMouseDown || type == kCGEventRightMouseDown || type == kCGEventOtherMouseDown)
			m->buttons[button] = 1;
		if (type == kCGEventLeftMouseUp || type == kCGEventRightMouseUp || type == kCGEventOtherMouseUp)
			m->buttons[button] = 0;
	}
	return event;
}

static ka_monitor *ka_monitor_open(void) {
	if (!CGPreflightListenEventAccess()) return NULL;
	ka_monitor *m = calloc(1, sizeof(ka_monitor));
	if (m == NULL) return NULL;
	m->tap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap,
		kCGEventTapOptionListenOnly, ka_monitor_mask(), ka_observe, m);
	if (m->tap == NULL) { free(m); return NULL; }
	// The OS may silently remove events from a requested mask. Verify that
	// keyboard and pointer coverage were both granted, without prompting.
	CGEventTapInformation taps[128];
	uint32_t count = 0;
	int complete = 0;
	if (CGGetEventTapList(128, taps, &count) == kCGErrorSuccess) {
		for (uint32_t i = 0; i < count; i++) {
			if (taps[i].tappingProcess == getpid() && taps[i].enabled &&
				(taps[i].eventsOfInterest & ka_monitor_mask()) == ka_monitor_mask()) complete = 1;
		}
	}
	m->source = CFMachPortCreateRunLoopSource(NULL, m->tap, 0);
	if (!complete || m->source == NULL) {
		if (m->source) CFRelease(m->source);
		CFMachPortInvalidate(m->tap); CFRelease(m->tap); free(m); return NULL;
	}
	// HIDSystemState can retain stale held-key bits after synthetic input.
	// The combined desktop session reflects current keys/buttons; from here
	// the tap tracks real transitions and excludes our own tagged events.
	for (int i = 0; i < 256; i++) m->keys[i] = i != 0x39 && CGEventSourceKeyState(kCGEventSourceStateCombinedSessionState, i);
	for (int i = 0; i < 32; i++) m->buttons[i] = CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, i);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), m->source, kCFRunLoopDefaultMode);
	m->healthy = 1;
	return m;
}

static int ka_monitor_held(ka_monitor *m) {
	for (int i = 0; i < 256; i++) if (m->keys[i]) return 1;
	for (int i = 0; i < 32; i++) if (m->buttons[i]) return 1;
	return 0;
}

static void ka_monitor_tick(ka_monitor *m) {
	CFRunLoopRunInMode(kCFRunLoopDefaultMode, 0.005, false);
	if (!CGEventTapIsEnabled(m->tap)) m->healthy = 0;
}

static void ka_monitor_close(ka_monitor *m) {
	CFRunLoopRemoveSource(CFRunLoopGetCurrent(), m->source, kCFRunLoopDefaultMode);
	CFMachPortInvalidate(m->tap);
	CFRelease(m->source);
	CFRelease(m->tap);
	free(m);
}
*/
import "C"

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

type cgMonitor struct {
	*inputState
	cancel context.CancelFunc
	done   chan struct{}
	pulse  atomic.Int64
}

func (*cgMonitor) Name() string { return "CoreGraphics user input" }

func (m *cgMonitor) Snapshot() inputSnapshot {
	if time.Since(time.Unix(0, m.pulse.Load())) > 100*time.Millisecond {
		m.fail(&Unavailable{Reason: "CoreGraphics input observer stopped responding"})
	}
	return m.inputState.Snapshot()
}

func (m *cgMonitor) Close() error {
	m.cancel()
	<-m.done
	return nil
}

func openCGMonitor(parent context.Context) (inputMonitor, error) {
	ctx, cancel := context.WithCancel(parent)
	m := &cgMonitor{inputState: newInputState(time.Now), cancel: cancel, done: make(chan struct{})}
	m.pulse.Store(time.Now().UnixNano())
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(m.done)
		native := C.ka_monitor_open()
		if native == nil {
			ready <- &Unavailable{Reason: "macOS did not grant complete keyboard and mouse monitoring", Hint: fmt.Sprintf("enable Input Monitoring for %s in System Settings → Privacy & Security; activity stays paused until monitoring works", quotedApp(responsibleApp()))}
			return
		}
		defer C.ka_monitor_close(native)
		if C.ka_monitor_held(native) != 0 {
			m.input(1, true)
		}
		m.pulse.Store(time.Now().UnixNano())
		ready <- nil
		var generation C.ulonglong
		for ctx.Err() == nil {
			C.ka_monitor_tick(native)
			m.pulse.Store(time.Now().UnixNano())
			if native.healthy == 0 {
				m.fail(&Unavailable{Reason: "macOS disabled user input monitoring", Hint: "check Input Monitoring permission; keepalive retries in 60 seconds"})
				return
			}
			if native.generation != generation {
				generation = native.generation
				m.input(1, C.ka_monitor_held(native) != 0)
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
