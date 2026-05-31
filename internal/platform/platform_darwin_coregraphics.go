//go:build darwin

package platform

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

static double keepalive_seconds_since_last_event() {
	return CGEventSourceSecondsSinceLastEventType(
		kCGEventSourceStateHIDSystemState,
		kCGAnyInputEventType
	);
}

static CGPoint keepalive_current_mouse_location() {
	CGEventRef event = CGEventCreate(NULL);
	if (event == NULL) {
		return CGPointMake(0, 0);
	}
	CGPoint point = CGEventGetLocation(event);
	CFRelease(event);
	return point;
}

static int keepalive_post_mouse_move(double x, double y) {
	CGPoint point = CGPointMake(x, y);
	CGEventRef event = CGEventCreateMouseEvent(
		NULL,
		kCGEventMouseMoved,
		point,
		kCGMouseButtonLeft
	);
	if (event != NULL) {
		CGEventPost(kCGHIDEventTap, event);
		CFRelease(event);
		return 1;
	}

	CGWarpMouseCursorPosition(point);
	return 1;
}
*/
import "C"

import (
	"fmt"
	"math"
	"time"
)

func coreGraphicsIdleTime() (time.Duration, error) {
	seconds := float64(C.keepalive_seconds_since_last_event())
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0, fmt.Errorf("invalid CoreGraphics idle time: %f", seconds)
	}

	return time.Duration(seconds * float64(time.Second)), nil
}

func coreGraphicsMouseLocation() (float64, float64, error) {
	point := C.keepalive_current_mouse_location()
	return float64(point.x), float64(point.y), nil
}

func coreGraphicsPostMouseMove(x, y float64) error {
	if C.keepalive_post_mouse_move(C.double(x), C.double(y)) == 0 {
		return fmt.Errorf("CoreGraphics failed to post mouse move")
	}
	return nil
}
