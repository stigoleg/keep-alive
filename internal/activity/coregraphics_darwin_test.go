//go:build darwin && cgo

package activity

import (
	"context"
	"os"
	"testing"
)

func TestAppNameFromPath(t *testing.T) {
	tests := map[string]string{
		"/Applications/Ghostty.app/Contents/MacOS/ghostty":                                               "Ghostty",
		"/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal":                            "Terminal",
		"/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper.app/Contents/MacOS/Helper": "Visual Studio Code",
		"/usr/local/bin/keepalive": "",
		"/weird/.app/bin":          "",
	}
	for path, want := range tests {
		if got := appNameFromPath(path); got != want {
			t.Errorf("appNameFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestAccessibilityHintNamesTheApp(t *testing.T) {
	u := accessibilityMissing()
	if u.Reason == "" || u.Hint == "" {
		t.Fatalf("accessibilityMissing() = %+v", u)
	}
	t.Logf("hint: %s", u.Hint)
	if got := quotedApp(""); got != "the app that launched keepalive" {
		t.Fatalf("quotedApp(\"\") = %q", got)
	}
}

// Reading counters, the pointer and the displays changes nothing.
func TestCoreGraphicsReadOnlyQueries(t *testing.T) {
	b := newBackend(context.Background(), false)
	for _, s := range append(b.sources, b.idle) {
		d, err := s.Idle()
		if err != nil || d < 0 {
			t.Fatalf("%s idle = %v, %v", s.Name(), d, err)
		}
	}
	if b.verify.Name() != "CombinedSessionState" {
		t.Fatalf("verifier = %s, want CombinedSessionState", b.verify.Name())
	}
	inj := cgInjector{}
	// Preflight only; it never prompts.
	t.Logf("post-event access: %v", inj.Available())
	x, y, ok := inj.Position()
	if !ok {
		t.Fatal("no pointer position")
	}
	r, ok := inj.Bounds()
	if !ok || !r.Contains(x, y) {
		t.Fatalf("bounds %+v (ok=%v) do not contain the pointer at (%v, %v)", r, ok, x, y)
	}
	if _, err := b.lock.Locked(); err != nil {
		t.Fatalf("lock state: %v", err)
	}
}

// TestProbeResetsCombinedSessionIdle moves the real pointer. Run it only
// while nobody is using the Mac:
//
//	KEEPALIVE_E2E_INPUT=1 go test -run TestProbeResetsCombinedSessionIdle -v ./internal/activity/
func TestProbeResetsCombinedSessionIdle(t *testing.T) {
	if os.Getenv("KEEPALIVE_E2E_INPUT") != "1" {
		t.Skip("moves the mouse; set KEEPALIVE_E2E_INPUT=1 to run")
	}
	res := Probe(context.Background(), false)
	for _, s := range res.Sources {
		t.Logf("%-22s before %-12v after %-12v %s", s.Source, s.Before, s.After, s.Err)
	}
	t.Logf("method=%q burst=%v locked=%v(known %v) effective=%v reason=%q hint=%q",
		res.Method, res.Burst, res.Locked, res.LockKnown, res.Effective, res.Reason, res.Hint)
	if res.Method != "CoreGraphics" {
		t.Fatalf("method = %q (%s; %s)", res.Method, res.Reason, res.Hint)
	}
	var combined *ProbeReading
	for i := range res.Sources {
		if res.Sources[i].Source == "CombinedSessionState" {
			combined = &res.Sources[i]
		}
	}
	if combined == nil || combined.Err != "" {
		t.Fatalf("no CombinedSessionState reading: %+v", res.Sources)
	}
	if combined.After >= effectiveIdle {
		t.Fatalf("CombinedSessionState idle after the burst = %v, want < %v", combined.After, effectiveIdle)
	}
	if !res.Effective {
		t.Fatalf("probe not effective: %s", res.Reason)
	}
}
