package notify

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

// TestNotifyArgsSignature checks the arguments against the Notify signature
// in the Desktop Notifications spec: (s app_name, u replaces_id, s app_icon,
// s summary, s body, as actions, a{sv} hints, i expire_timeout).
func TestNotifyArgsSignature(t *testing.T) {
	args := notifyArgs("keepalive", "notification test")
	if got := dbus.SignatureOf(args...).String(); got != "susssasa{sv}i" {
		t.Fatalf("Notify signature = %s, want susssasa{sv}i", got)
	}
	if args[0] != "keepalive" || args[3] != "keepalive" || args[4] != "notification test" || args[7] != int32(-1) {
		t.Fatalf("Notify args = %#v", args)
	}
}
