// Package legacy shares one v1 platform.KeepAlive between the v2 power and
// activity adapters. The v1 object couples the power assertion with the
// jitter loop, so both adapters reference-count a single instance. Phases 2-3
// delete this package together with the adapters.
package legacy

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"github.com/stigoleg/keep-alive/v2/internal/platform"
)

type object struct {
	mu            sync.Mutex
	ka            platform.KeepAlive
	refs          int
	activityUsers int
}

var (
	shared       object
	newKeepAlive = platform.NewKeepAlive
)

// Acquire starts the shared platform object on the first reference.
func Acquire() error {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	if shared.refs == 0 {
		ka, err := newKeepAlive()
		if err != nil {
			return err
		}
		ka.SetSimulateActivity(shared.activityUsers > 0)
		if err := ka.Start(context.Background()); err != nil {
			return err
		}
		shared.ka = ka
	}
	shared.refs++
	return nil
}

// Release stops the shared platform object when the last reference goes.
func Release() error {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	if shared.refs == 0 {
		return errors.New("legacy: release without acquire")
	}
	shared.refs--
	if shared.refs > 0 {
		return nil
	}
	ka := shared.ka
	shared.ka = nil
	return ka.Stop()
}

// SetActivity registers (on) or unregisters (off) an activity user. Jitter is
// enabled while at least one user is registered, so an old simulator exiting
// after a new one started cannot switch simulation off.
func SetActivity(on bool) {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	before := shared.activityUsers > 0
	if on {
		shared.activityUsers++
	} else if shared.activityUsers > 0 {
		shared.activityUsers--
	}
	after := shared.activityUsers > 0
	if before != after && shared.ka != nil {
		shared.ka.SetSimulateActivity(after)
	}
}

// Observe registers fn for platform activity-controller events until the
// returned cancel func is called. It fans the single platform observer out so
// overlapping simulators do not unregister each other.
func Observe(fn func(platform.ActivityEvent)) (cancel func()) {
	observers.mu.Lock()
	defer observers.mu.Unlock()
	observers.next++
	id := observers.next
	if observers.fns == nil {
		observers.fns = map[int]func(platform.ActivityEvent){}
	}
	observers.fns[id] = fn
	if len(observers.fns) == 1 {
		platform.SetActivityObserver(fanOut)
	}
	return func() {
		observers.mu.Lock()
		defer observers.mu.Unlock()
		if _, ok := observers.fns[id]; !ok {
			return
		}
		delete(observers.fns, id)
		if len(observers.fns) == 0 {
			platform.SetActivityObserver(nil)
		}
	}
}

var observers struct {
	mu   sync.Mutex
	next int
	fns  map[int]func(platform.ActivityEvent)
}

func fanOut(ev platform.ActivityEvent) {
	observers.mu.Lock()
	fns := make([]func(platform.ActivityEvent), 0, len(observers.fns))
	for _, fn := range observers.fns {
		fns = append(fns, fn)
	}
	observers.mu.Unlock()
	for _, fn := range fns {
		fn(ev)
	}
}

// Describe names the v1 mechanism holding the system awake.
func Describe() string {
	switch runtime.GOOS {
	case "darwin":
		return "caffeinate -dims"
	case "windows":
		return "SetThreadExecutionState"
	case "linux":
		return "systemd/D-Bus inhibitors"
	default:
		return "unsupported platform"
	}
}
