package activity

import (
	"strings"
	"time"
)

// Diagnostics is what activity simulation can use on this machine, found
// with cheap read-only checks: no pointer movement, no permission prompt.
type Diagnostics struct {
	// Injectors are the input methods in the order they are tried.
	Injectors []InjectorCheck
	// Idle holds every idle counter that answered.
	Idle []IdleCheck
	// Gate is the idle source that decides when to simulate; empty means a
	// fixed schedule (see NoIdleHint).
	Gate string
	// Verifier is the idle source bursts are judged by (the one chat apps
	// read).
	Verifier   string
	NoIdleHint string
	Lock       LockCheck
	// Environment describes the desktop, e.g. "display server: Wayland".
	Environment []string
}

// InjectorCheck is one input method.
type InjectorCheck struct {
	Name      string
	Available bool
	// Detail adds context when available, e.g. who holds the permission.
	Detail       string
	Reason, Hint string
}

// IdleCheck is one idle counter reading.
type IdleCheck struct {
	Name string
	Idle time.Duration
}

// LockCheck describes screen-lock detection. An empty Method means it is not
// available.
type LockCheck struct {
	Method        string
	Known, Locked bool
	Err           string
}

// Usable reports whether any input method is available.
func (d Diagnostics) Usable() bool {
	for _, c := range d.Injectors {
		if c.Available {
			return true
		}
	}
	return false
}

// Problem explains why no input method is usable; empty when one is.
func (d Diagnostics) Problem() (reason, hint string) {
	if d.Usable() {
		return "", ""
	}
	var reasons []string
	for _, c := range d.Injectors {
		if hint == "" {
			hint = c.Hint
		}
		if len(d.Injectors) == 1 {
			reasons = append(reasons, c.Reason)
		} else {
			reasons = append(reasons, c.Name+": "+c.Reason)
		}
	}
	if len(reasons) == 0 {
		return "no input method on this system", hint
	}
	return strings.Join(reasons, "; "), hint
}

// Diagnose inspects the activity backends without simulating anything.
func Diagnose() Diagnostics {
	b := newBackend(false)
	defer b.Close()
	return diagnose(b)
}

func diagnose(b *backend) Diagnostics {
	d := Diagnostics{NoIdleHint: b.noIdleHint, Environment: b.env}
	if b.candidates != nil {
		for _, inj := range b.candidates() {
			c := InjectorCheck{Name: inj.Name()}
			if err := inj.Available(); err != nil {
				c.Reason, c.Hint = explain(err)
			} else {
				c.Available = true
				if dt, ok := inj.(interface{ Detail() string }); ok {
					c.Detail = dt.Detail()
				}
			}
			d.Injectors = append(d.Injectors, c)
		}
	} else if inj, err := b.open(); err != nil {
		// Only backends whose open has no side effects leave candidates nil.
		c := InjectorCheck{Name: "input"}
		c.Reason, c.Hint = explain(err)
		d.Injectors = append(d.Injectors, c)
	} else {
		d.Injectors = append(d.Injectors, InjectorCheck{Name: inj.Name(), Available: true})
		_ = inj.Close()
	}

	for _, s := range b.sources {
		if idle, err := s.Idle(); err == nil {
			d.Idle = append(d.Idle, IdleCheck{Name: s.Name(), Idle: idle})
		}
	}
	if b.idle != nil {
		d.Gate = b.idle.Name()
	}
	if v := b.verify; v != nil {
		d.Verifier = v.Name()
	} else if b.idle != nil {
		d.Verifier = b.idle.Name()
	}
	if b.lock != nil {
		d.Lock.Method = b.lockName
		if d.Lock.Method == "" {
			d.Lock.Method = "available"
		}
		locked, err := b.lock.Locked()
		d.Lock.Known, d.Lock.Locked = err == nil, err == nil && locked
		if err != nil {
			d.Lock.Err = err.Error()
		}
	}
	return d
}
