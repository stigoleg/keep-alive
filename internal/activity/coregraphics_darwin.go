//go:build darwin && cgo

package activity

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <dlfcn.h>
#include <libproc.h>
#include <pthread.h>
#include <unistd.h>

static CGEventSourceRef ka_source = NULL;
static pthread_once_t ka_source_once = PTHREAD_ONCE_INIT;

// Events from a HIDSystemState source reset the HID and the combined-session
// idle counters; the latter is what Chromium/Electron (Teams, Slack) read.
static void ka_init_source(void) {
	ka_source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
}

static CGEventSourceRef ka_get_source(void) {
	pthread_once(&ka_source_once, ka_init_source);
	return ka_source;
}

static double ka_idle(int combined) {
	return CGEventSourceSecondsSinceLastEventType(
		combined ? kCGEventSourceStateCombinedSessionState : kCGEventSourceStateHIDSystemState,
		kCGAnyInputEventType);
}

static int ka_preflight(void) { return CGPreflightPostEventAccess() ? 1 : 0; }

static int ka_request(void) { return CGRequestPostEventAccess() ? 1 : 0; }

static int ka_location(double *x, double *y) {
	CGEventRef e = CGEventCreate(NULL);
	if (e == NULL) {
		return 0;
	}
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	*x = p.x;
	*y = p.y;
	return 1;
}

// ka_display_bounds returns the display containing (px, py), or the union of
// all active displays when none does.
static int ka_display_bounds(double px, double py, double *x, double *y, double *w, double *h) {
	CGDirectDisplayID ids[32];
	uint32_t n = 0;
	if (CGGetActiveDisplayList(32, ids, &n) != kCGErrorSuccess || n == 0) {
		return 0;
	}
	CGRect r = CGRectNull;
	for (uint32_t i = 0; i < n; i++) {
		CGRect b = CGDisplayBounds(ids[i]);
		if (CGRectContainsPoint(b, CGPointMake(px, py))) {
			r = b;
			break;
		}
		r = CGRectUnion(r, b);
	}
	*x = r.origin.x;
	*y = r.origin.y;
	*w = r.size.width;
	*h = r.size.height;
	return 1;
}

static int ka_move(double x, double y) {
	CGEventSourceRef src = ka_get_source();
	if (src == NULL) {
		return 0;
	}
	CGEventRef e = CGEventCreateMouseEvent(src, kCGEventMouseMoved, CGPointMake(x, y), kCGMouseButtonLeft);
	if (e == NULL) {
		return 0;
	}
	CGEventSetIntegerValueField(e, kCGEventSourceUserData, (int64_t)getpid());
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
	return 1;
}

// ka_tap_right_shift presses and releases right Shift (kVK_RightShift) as
// the modifier-change events a real keyboard produces.
static int ka_tap_right_shift(void) {
	CGEventSourceRef src = ka_get_source();
	if (src == NULL) {
		return 0;
	}
	CGEventRef down = CGEventCreateKeyboardEvent(src, (CGKeyCode)0x3C, true);
	CGEventRef up = CGEventCreateKeyboardEvent(src, (CGKeyCode)0x3C, false);
	if (down == NULL || up == NULL) {
		if (down != NULL) CFRelease(down);
		if (up != NULL) CFRelease(up);
		return 0;
	}
	// 0x4 is NX_DEVICERSHIFTKEYMASK, the right-Shift device bit.
	CGEventSetType(down, kCGEventFlagsChanged);
	CGEventSetFlags(down, kCGEventFlagMaskShift | kCGEventFlagMaskNonCoalesced | 0x4);
	CGEventSetType(up, kCGEventFlagsChanged);
	CGEventSetFlags(up, kCGEventFlagMaskNonCoalesced);
	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(down);
	CFRelease(up);
	return 1;
}

// ka_responsible_pid asks the private responsibility API which process TCC
// charges our input to (the terminal app when run from a shell). It is
// looked up at run time so a missing symbol only loses the app name.
typedef pid_t (*ka_responsible_fn)(pid_t);

static int ka_responsible_pid(void) {
	ka_responsible_fn f = (ka_responsible_fn)dlsym(RTLD_DEFAULT, "responsibility_get_pid_responsible_for_pid");
	if (f == NULL) {
		return -1;
	}
	return (int)f(getpid());
}

static int ka_pid_path(int pid, char *buf, int size) {
	return proc_pidpath(pid, buf, (uint32_t)size);
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
)

func newBackend(_ context.Context, keys bool) *backend {
	hid, combined := cgIdle{combined: false}, cgIdle{combined: true}
	return &backend{
		idle:    maxIdle{hid, combined},
		verify:  combined,
		sources: []IdleSource{hid, combined},
		lock:    sessionLock{},
		open:    openCoreGraphics,

		candidates: func() []Injector { return []Injector{cgInjector{}} },
		lockName:   "session dictionary, polled every 2 s",
	}
}

// cgIdle reads one CoreGraphics idle counter. Electron's
// powerMonitor.getSystemIdleTime() reads the combined-session one.
type cgIdle struct{ combined bool }

func (s cgIdle) Name() string {
	if s.combined {
		return "CombinedSessionState"
	}
	return "HIDSystemState"
}

func (s cgIdle) Idle() (time.Duration, error) {
	c := 0
	if s.combined {
		c = 1
	}
	secs := float64(C.ka_idle(C.int(c)))
	if math.IsNaN(secs) || math.IsInf(secs, 0) || secs < 0 {
		return 0, fmt.Errorf("invalid %s idle time %v", s.Name(), secs)
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// maxIdle gates on the larger of several counters, so real input on any of
// them counts as the user being present.
type maxIdle []IdleSource

func (maxIdle) Name() string { return "CoreGraphics" }

func (m maxIdle) Idle() (time.Duration, error) {
	var out time.Duration
	for _, s := range m {
		d, err := s.Idle()
		if err != nil {
			return 0, err
		}
		out = max(out, d)
	}
	return out, nil
}

var errPostFailed = errors.New("CoreGraphics could not create the event")

// requestAccess prompts for the Accessibility permission at most once per
// process; macOS shows the prompt only the first time anyway.
var requestAccess sync.Once

func openCoreGraphics() (Injector, error) {
	inj := cgInjector{}
	if inj.Available() == nil {
		return inj, nil
	}
	granted := false
	requestAccess.Do(func() { granted = C.ka_request() == 1 })
	if granted {
		return inj, nil
	}
	return nil, accessibilityMissing()
}

// cgInjector posts mouse-moved events at absolute positions through the HID
// event tap.
type cgInjector struct{}

func (cgInjector) Name() string { return "CoreGraphics" }

func (cgInjector) Available() error {
	if C.ka_preflight() == 1 {
		return nil
	}
	return accessibilityMissing()
}

func (cgInjector) MoveTo(x, y float64) error {
	if C.ka_move(C.double(x), C.double(y)) == 0 {
		return errPostFailed
	}
	return nil
}

func (cgInjector) Position() (float64, float64, bool) {
	var x, y C.double
	if C.ka_location(&x, &y) == 0 {
		return 0, 0, false
	}
	return float64(x), float64(y), true
}

func (i cgInjector) Bounds() (Rect, bool) {
	px, py, ok := i.Position()
	if !ok {
		return Rect{}, false
	}
	var x, y, w, h C.double
	if C.ka_display_bounds(C.double(px), C.double(py), &x, &y, &w, &h) == 0 {
		return Rect{}, false
	}
	return Rect{X: float64(x), Y: float64(y), W: float64(w), H: float64(h)}, true
}

func (cgInjector) Tap() error {
	if C.ka_tap_right_shift() == 0 {
		return errPostFailed
	}
	return nil
}

func (cgInjector) Diagnose() (string, string) {
	if C.ka_preflight() != 1 {
		u := accessibilityMissing()
		return u.Reason, u.Hint
	}
	return "macOS ignored the synthetic input",
		fmt.Sprintf("check that %s is switched on under System Settings → Privacy & Security → Accessibility; after an update, remove the entry and add it again", quotedApp(responsibleApp()))
}

func (cgInjector) Close() error { return nil }

// Detail names the app that holds the Accessibility permission.
func (cgInjector) Detail() string {
	return "Accessibility granted to " + quotedApp(responsibleApp())
}

func accessibilityMissing() *Unavailable {
	return &Unavailable{
		Reason: "Accessibility permission is missing",
		Hint: fmt.Sprintf("Grant Accessibility to %s in System Settings → Privacy & Security → Accessibility, then keep keepalive running (it rechecks every 60 s)",
			quotedApp(responsibleApp())),
	}
}

func quotedApp(name string) string {
	if name == "" {
		return "the app that launched keepalive"
	}
	return `"` + name + `"`
}

// responsibleApp names the app macOS asks Accessibility permission for: the
// terminal when keepalive runs in a shell, keepalive itself under launchd.
func responsibleApp() string {
	pid := int(C.ka_responsible_pid())
	if pid > 0 {
		if path := pidPath(pid); path != "" {
			if name := appNameFromPath(path); name != "" {
				return name
			}
			if pid == os.Getpid() {
				return path
			}
		}
	}
	return os.Getenv("TERM_PROGRAM")
}

func pidPath(pid int) string {
	buf := make([]byte, 4096) // PROC_PIDPATHINFO_MAXSIZE
	n := C.ka_pid_path(C.int(pid), (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}

// appNameFromPath returns the outermost .app bundle name in path, e.g.
// "Ghostty" for /Applications/Ghostty.app/Contents/MacOS/ghostty.
func appNameFromPath(path string) string {
	for _, part := range strings.Split(path, "/") {
		if name, ok := strings.CutSuffix(part, ".app"); ok && name != "" {
			return name
		}
	}
	return ""
}
