package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/muesli/termenv"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// t0 is on a bucket edge: 15:48:00.
var t0 = time.Date(2026, 10, 5, 15, 48, 0, 0, time.UTC)

func TestBurstBucketsFollowTheClock(t *testing.T) {
	now := t0.Add(10 * time.Second) // 15:48:10, the current bucket is 15:48:00–15:48:30
	b := newBurstLog(t0.Add(-time.Hour))
	for _, at := range []time.Duration{
		0,                 // the current bucket's first instant
		-time.Nanosecond,  // the previous bucket's last instant
		-30 * time.Second, // the previous bucket's first instant
		-31 * time.Second, // two back
		-330 * time.Second,
		-331 * time.Second, // before the oldest bucket (15:42:30): dropped
		20 * time.Second,   // ahead of now: counts as now
	} {
		b.observe(t0.Add(at), now)
	}
	want := []int{1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 2}
	if got := b.counts(now); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("counts %v, want %v", got, want)
	}
	// 30 s later everything moves one bucket to the left, and the oldest
	// falls out.
	later := now.Add(sparkBucket)
	want = []int{0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 2, 0}
	if got := b.counts(later); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("30 s later: counts %v, want %v", got, want)
	}
}

func TestBurstLogRecordsEachBurstOnce(t *testing.T) {
	b := newBurstLog(t0)
	burst := t0.Add(5 * time.Second)
	for range 5 { // the same LastBurst in five snapshots
		b.observe(burst, t0.Add(10*time.Second))
	}
	b.observe(time.Time{}, t0.Add(10*time.Second))
	if len(b.times) != 1 {
		t.Fatalf("%d bursts recorded", len(b.times))
	}
}

func TestBurstLogIsBounded(t *testing.T) {
	b := newBurstLog(t0)
	now := t0
	for i := range 1000 {
		now = t0.Add(time.Duration(i) * time.Second)
		b.observe(now, now)
	}
	if len(b.times) > maxBursts {
		t.Fatalf("%d bursts kept", len(b.times))
	}
	for _, at := range b.times {
		if at.Before(windowStart(now)) {
			t.Fatalf("kept %v, older than the sparkline", at)
		}
	}
	// A slow rate keeps only what the sparkline shows.
	b = newBurstLog(t0)
	for i := range 100 {
		now = t0.Add(time.Duration(i) * time.Minute)
		b.observe(now, now)
	}
	if len(b.times) != 6 {
		t.Fatalf("%d bursts kept, want the last 6 minutes' 6", len(b.times))
	}
}

func TestAttachedSparklineStartsEmpty(t *testing.T) {
	snap := timedSnap()
	snap.Activity = activity.Status{State: activity.StateSimulating, LastBurst: testNow.Add(-20 * time.Second)}
	h, c := attachedHarness(t, snap, 64)
	h.update(snapMsg{c: c, snap: snap})
	if got := h.m.dash.bursts.counts(testNow); fmt.Sprint(got) != fmt.Sprint(make([]int, sparkBuckets)) {
		t.Fatalf("a burst from before attaching shows: %v", got)
	}
	snap.Activity.LastBurst = testNow
	h.update(snapMsg{c: c, snap: snap})
	if got := h.m.dash.bursts.counts(testNow); got[sparkBuckets-1] != 1 {
		t.Fatalf("a burst after attaching is missing: %v", got)
	}
	// Events carry bursts too.
	snap.Activity.LastBurst = testNow.Add(time.Second)
	h.update(eventMsg{c: c, ok: true, ev: session.Event{Type: session.EventActivity, Snapshot: snap}})
	if got := h.m.dash.bursts.counts(testNow.Add(time.Second)); got[sparkBuckets-1] != 2 {
		t.Fatalf("a burst from an event is missing: %v", got)
	}
}

// pulseHarness is a dashboard of a local session in colour showing snap.
func pulseHarness(t *testing.T, snap session.Snapshot) (*harness, *fakeCtrl) {
	t.Helper()
	st := &starter{}
	o := testOptions(st)
	o.Renderer = forced(termenv.TrueColor, true)
	o.Base, o.Start = session.Config{Active: true}, true
	h := newHarness(t, o, 64)
	c := st.last(t)
	h.update(snapMsg{c: c, snap: snap})
	return h, c
}

func simulating() session.Snapshot {
	s := runningSnap(session.Config{Active: true})
	s.Activity = activity.Status{State: activity.StateSimulating, LastBurst: testNow}
	return s
}

func TestPulseRunsOnlyWhileSimulatingAndVisible(t *testing.T) {
	waiting := runningSnap(session.Config{Active: true})
	h, c := pulseHarness(t, waiting)
	if h.m.pulsing {
		t.Fatal("pulse runs while waiting for idle")
	}

	// Simulating: the pulse starts, and each tick flips the mark and asks
	// for the next.
	m, cmd := h.m.Update(snapMsg{c: c, snap: simulating()})
	h.m = m.(Model)
	if !h.m.pulsing || cmd == nil {
		t.Fatal("pulse did not start while simulating")
	}
	gen := h.m.pulseGen
	m, cmd = h.m.Update(pulseMsg{gen: gen})
	h.m = m.(Model)
	if !h.m.pulseDim || cmd == nil {
		t.Fatalf("tick: dim %v, next %v", h.m.pulseDim, cmd != nil)
	}

	// Help hides the dashboard: the pulse stops, and the tick on its way
	// asks for no other.
	h.press("?")
	if h.m.pulsing {
		t.Fatal("pulse runs under help")
	}
	if _, cmd := h.m.Update(pulseMsg{gen: gen}); cmd != nil {
		t.Fatal("a stale tick continues the pulse")
	}
	h.press("?")
	if !h.m.pulsing {
		t.Fatal("pulse did not resume after help")
	}

	// Paused by the user: no pulse.
	paused := simulating()
	paused.Activity.State = activity.StatePausedUser
	h.update(snapMsg{c: c, snap: paused})
	if h.m.pulsing {
		t.Fatal("pulse runs while paused")
	}
	if _, cmd := h.m.Update(pulseMsg{gen: h.m.pulseGen}); cmd != nil {
		t.Fatal("tick while paused asks for another")
	}

	// Simulating again, then stopped: Home has no pulse.
	h.update(snapMsg{c: c, snap: simulating()})
	if !h.m.pulsing {
		t.Fatal("pulse did not restart")
	}
	h.press("s")
	h.settle()
	if h.m.screen != screenHome || h.m.pulsing {
		t.Fatalf("screen %v pulsing %v", h.m.screen, h.m.pulsing)
	}
}

func TestNoPulseWithoutColour(t *testing.T) {
	st := &starter{}
	o := testOptions(st) // plain text
	o.Base, o.Start = session.Config{Active: true}, true
	h := newHarness(t, o, 64)
	h.update(snapMsg{c: st.last(t), snap: simulating()})
	if h.m.pulsing {
		t.Fatal("pulse runs without colours")
	}
}
