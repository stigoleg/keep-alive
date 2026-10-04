//go:build darwin && cgo

package activity

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>

static int ka_dict_flag(CFDictionaryRef d, CFStringRef key, int missing) {
	CFTypeRef v = CFDictionaryGetValue(d, key);
	if (v == NULL) {
		return missing;
	}
	if (CFGetTypeID(v) == CFBooleanGetTypeID()) {
		return CFBooleanGetValue((CFBooleanRef)v) ? 1 : 0;
	}
	if (CFGetTypeID(v) == CFNumberGetTypeID()) {
		int n = 0;
		CFNumberGetValue((CFNumberRef)v, kCFNumberIntType, &n);
		return n != 0;
	}
	return missing;
}

// ka_session_locked: 1 when the screen is locked or our session is not the
// one on the console (fast user switching), 0 otherwise, -1 without a GUI
// session. CGSSessionScreenIsLocked is undocumented and absent while
// unlocked.
static int ka_session_locked(void) {
	CFDictionaryRef d = CGSessionCopyCurrentDictionary();
	if (d == NULL) {
		return -1;
	}
	int locked = ka_dict_flag(d, CFSTR("CGSSessionScreenIsLocked"), 0);
	if (!ka_dict_flag(d, kCGSessionOnConsoleKey, 1)) {
		locked = 1;
	}
	CFRelease(d);
	return locked;
}
*/
import "C"

import "errors"

// sessionLock polls the window server's session dictionary.
//
// Chromium watches the com.apple.screenIsLocked distributed notification
// instead, but CoreFoundation delivers distributed notifications only
// through the main thread's run loop, whatever thread registers the
// observer; keepalive's main thread runs the CLI, so an observer would
// never fire. Polling every tick is cheap and also sees a screen that was
// locked before keepalive started.
type sessionLock struct{}

func (sessionLock) Locked() (bool, error) {
	switch C.ka_session_locked() {
	case 1:
		return true, nil
	case 0:
		return false, nil
	}
	return false, errors.New("no window server session")
}
