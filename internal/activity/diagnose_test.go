package activity

import (
	"errors"
	"testing"
	"time"
)

type unavailableInjector struct {
	fakeInjector
	err error
}

func (unavailableInjector) Name() string       { return "Other" }
func (u unavailableInjector) Available() error { return u.err }

func TestDiagnoseIsStatic(t *testing.T) {
	m := newMachine()
	m.clk.Advance(90 * time.Second)
	m.locked = true
	b := probeBackend(m)
	b.lockName = "fake lock"
	b.env = []string{"desktop: fake"}
	b.candidates = func() []Injector {
		return []Injector{
			fakeInjector{m},
			unavailableInjector{err: &Unavailable{Reason: "not installed", Hint: "install it"}},
			unavailableInjector{err: errors.New("plain failure")},
		}
	}
	b.open = func() (Injector, error) {
		t.Fatal("Diagnose opened an injector")
		return nil, nil
	}

	d := diagnose(b)
	if len(d.Injectors) != 3 {
		t.Fatalf("injectors = %+v", d.Injectors)
	}
	if got := d.Injectors[0]; got.Name != "Fake" || !got.Available || got.Reason != "" {
		t.Fatalf("first injector = %+v", got)
	}
	if got := d.Injectors[1]; got.Available || got.Reason != "not installed" || got.Hint != "install it" {
		t.Fatalf("second injector = %+v", got)
	}
	if got := d.Injectors[2]; got.Available || got.Reason != "plain failure" || got.Hint != "" {
		t.Fatalf("third injector = %+v", got)
	}
	if len(d.Idle) != 1 || d.Idle[0].Name != "fake idle" || d.Idle[0].Idle != 90*time.Second {
		t.Fatalf("idle = %+v", d.Idle)
	}
	if d.Gate != "fake idle" || d.Verifier != "fake idle" {
		t.Fatalf("gate %q verifier %q", d.Gate, d.Verifier)
	}
	if d.Lock.Method != "fake lock" || !d.Lock.Known || !d.Lock.Locked {
		t.Fatalf("lock = %+v", d.Lock)
	}
	if len(d.Environment) != 1 || d.Environment[0] != "desktop: fake" {
		t.Fatalf("environment = %v", d.Environment)
	}
	if !d.Usable() {
		t.Fatal("Usable() = false with an available injector")
	}
	if m.burstCount() != 0 || m.taps != 0 {
		t.Fatal("Diagnose produced input")
	}
}

func TestDiagnoseWithoutCandidatesUsesOpen(t *testing.T) {
	b := &backend{
		noIdleHint: "install an idle source",
		open: func() (Injector, error) {
			return nil, &Unavailable{Reason: "unavailable: built without cgo", Hint: "rebuild"}
		},
	}
	d := diagnose(b)
	if len(d.Injectors) != 1 || d.Injectors[0].Available || d.Injectors[0].Reason != "unavailable: built without cgo" {
		t.Fatalf("injectors = %+v", d.Injectors)
	}
	if d.Usable() {
		t.Fatal("Usable() = true without an injector")
	}
	if d.Gate != "" || d.NoIdleHint != "install an idle source" || d.Lock.Method != "" {
		t.Fatalf("diagnostics = %+v", d)
	}
	if reason, hint := d.Problem(); reason != "unavailable: built without cgo" || hint != "rebuild" {
		t.Fatalf("Problem() = %q, %q", reason, hint)
	}
}
