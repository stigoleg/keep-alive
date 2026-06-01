//go:build darwin

package platform

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <pthread.h>
#include <unistd.h>

static CGEventSourceRef kaSource = NULL;
static pthread_once_t kaSourceOnce = PTHREAD_ONCE_INIT;

// HIDSystemState updates both the HID and combined-session counters; the
// latter is what Chromium/Electron (Teams, Slack) read for idle detection.
static void keepalive_init_source(void) {
	kaSource = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
}

static CGEventSourceRef keepalive_source(void) {
	pthread_once(&kaSourceOnce, keepalive_init_source);
	return kaSource;
}

static double keepalive_idle_hid(void) {
	return CGEventSourceSecondsSinceLastEventType(
		kCGEventSourceStateHIDSystemState,
		kCGAnyInputEventType);
}

static double keepalive_idle_combined(void) {
	return CGEventSourceSecondsSinceLastEventType(
		kCGEventSourceStateCombinedSessionState,
		kCGAnyInputEventType);
}

static int keepalive_preflight_post_access(void) {
	return CGPreflightPostEventAccess() ? 1 : 0;
}

static int keepalive_request_post_access(void) {
	return CGRequestPostEventAccess() ? 1 : 0;
}

static CGPoint keepalive_current_mouse_location(void) {
	CGEventRef event = CGEventCreate(NULL);
	if (event == NULL) {
		return CGPointMake(0, 0);
	}
	CGPoint point = CGEventGetLocation(event);
	CFRelease(event);
	return point;
}

static int keepalive_post_mouse_move(double x, double y) {
	CGEventSourceRef src = keepalive_source();
	if (src == NULL) {
		return 0;
	}
	CGPoint point = CGPointMake(x, y);
	CGEventRef event = CGEventCreateMouseEvent(
		src,
		kCGEventMouseMoved,
		point,
		kCGMouseButtonLeft);
	if (event == NULL) {
		return 0;
	}
	CGEventSetIntegerValueField(event, kCGEventSourceUserData, (int64_t)getpid());
	CGEventPost(kCGHIDEventTap, event);
	CFRelease(event);
	return 1;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// errCoreGraphicsPostFailed is returned when CGEventPost cannot deliver a
// synthetic event — typically because Accessibility permission is missing.
var errCoreGraphicsPostFailed = errors.New("CoreGraphics failed to post mouse event")

func coreGraphicsIdleTime() (time.Duration, error) {
	hid := float64(C.keepalive_idle_hid())
	combined := float64(C.keepalive_idle_combined())

	seconds := math.Max(hid, combined)
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0, fmt.Errorf("invalid CoreGraphics idle time (hid=%f combined=%f)", hid, combined)
	}

	return time.Duration(seconds * float64(time.Second)), nil
}

func coreGraphicsMouseLocation() (float64, float64, error) {
	point := C.keepalive_current_mouse_location()
	return float64(point.x), float64(point.y), nil
}

func coreGraphicsPostMouseMove(x, y float64) error {
	if C.keepalive_post_mouse_move(C.double(x), C.double(y)) == 0 {
		return errCoreGraphicsPostFailed
	}
	return nil
}

// coreGraphicsPreflightPostAccess reports whether the current process is
// trusted to post synthetic events. Does not prompt.
func coreGraphicsPreflightPostAccess() bool {
	return C.keepalive_preflight_post_access() == 1
}

// coreGraphicsRequestPostAccess triggers the system Accessibility prompt on
// first call. Returns true if access is already (or now) granted.
func coreGraphicsRequestPostAccess() bool {
	return C.keepalive_request_post_access() == 1
}
