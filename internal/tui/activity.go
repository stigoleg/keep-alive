package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
)

// The sparkline shows the last sparkBuckets × sparkBucket of bursts, one
// cell per bucket. Buckets follow the wall clock (:00 and :30), so the
// cells move one step every 30 s rather than smearing.
const (
	sparkBuckets = 12
	sparkBucket  = 30 * time.Second
	// maxBursts bounds the history whatever the burst rate.
	maxBursts = 64
	// pulseEvery is how often the simulating mark changes colour.
	pulseEvery = 800 * time.Millisecond
)

// burstLog is the recent bursts of the followed session, built from each
// change of Activity.LastBurst in its snapshots and events. The session
// reports only its last burst, so a burst between two observations is
// missed; with a burst every 30 s or more and a snapshot every second that
// does not happen.
type burstLog struct {
	since time.Time   // bursts before this are not ours to show (attach)
	last  time.Time   // the last LastBurst seen
	times []time.Time // oldest first
}

func newBurstLog(since time.Time) burstLog { return burstLog{since: since} }

// observe records burst t once.
func (b *burstLog) observe(t, now time.Time) {
	if t.IsZero() || t.Equal(b.last) {
		return
	}
	b.last = t
	if t.Before(b.since) {
		return
	}
	b.times = append(b.times, t)
	b.prune(now)
}

// prune drops bursts older than the sparkline and keeps at most maxBursts.
func (b *burstLog) prune(now time.Time) {
	oldest := windowStart(now)
	i := 0
	for i < len(b.times) && b.times[i].Before(oldest) {
		i++
	}
	if n := len(b.times) - i; n > maxBursts {
		i += n - maxBursts
	}
	if i > 0 {
		b.times = append([]time.Time(nil), b.times[i:]...)
	}
}

// windowStart is the start of the oldest bucket shown at now.
func windowStart(now time.Time) time.Time {
	return now.Truncate(sparkBucket).Add(-(sparkBuckets - 1) * sparkBucket)
}

// counts returns the bursts per bucket at now, oldest first; the last
// bucket is the current one. A burst ahead of now counts as now.
func (b burstLog) counts(now time.Time) []int {
	out := make([]int, sparkBuckets)
	cur := now.Truncate(sparkBucket)
	for _, t := range b.times {
		k := 0
		if t.Before(cur) {
			k = int(cur.Sub(t.Truncate(sparkBucket)) / sparkBucket)
		}
		if k < sparkBuckets {
			out[sparkBuckets-1-k]++
		}
	}
	return out
}

// ---- pulse ----

type pulseMsg struct{ gen int }

func pulseCmd(gen int) tea.Cmd {
	return tea.Tick(pulseEvery, func(time.Time) tea.Msg { return pulseMsg{gen: gen} })
}

// wantPulse says whether the simulating mark should pulse: only while the
// dashboard is visible, the session simulates activity and there are
// colours to pulse with.
func (m Model) wantPulse() bool {
	s := m.dash.snap
	return m.st.color && m.screen == screenDashboard && !m.help && m.dash.ended == "" &&
		s.Running && s.Active && s.InWindow && s.Paused == "" && s.Activity.State == activity.StateSimulating
}

// syncPulse starts the pulse tick when it is wanted and not running, and
// ends it otherwise: a tick already on its way is then ignored.
func (m Model) syncPulse() (Model, tea.Cmd) {
	want := m.wantPulse()
	switch {
	case want && !m.pulsing:
		m.pulsing, m.pulseDim = true, false
		m.pulseGen++
		return m, pulseCmd(m.pulseGen)
	case !want && m.pulsing:
		m.pulsing, m.pulseDim = false, false
		m.pulseGen++
	}
	return m, nil
}

func (m Model) onPulse(msg pulseMsg) (Model, tea.Cmd) {
	if msg.gen != m.pulseGen || !m.pulsing {
		return m, nil
	}
	m.pulseDim = !m.pulseDim
	return m, pulseCmd(m.pulseGen)
}
