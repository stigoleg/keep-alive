package tui

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

func TestMain(m *testing.M) {
	// Never run the real activity diagnostics, and keep the control socket
	// of the attach tests away from a real keepalive.
	diagnoseActivity = func() (string, string) { return "", "" }
	dir, err := os.MkdirTemp("", "kt")
	if err != nil {
		panic(err)
	}
	os.Setenv(ipc.EnvRuntimeDir, dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// testNow is a Monday afternoon.
var testNow = fixtureNow

// fakeCtrl is a Controller that records calls.
type fakeCtrl struct {
	mu     sync.Mutex
	snap   session.Snapshot
	events chan session.Event
	calls  []string
	inst   *Instance
	cfg    session.Config
	ended  bool
}

func newFakeCtrl(snap session.Snapshot, inst *Instance) *fakeCtrl {
	return &fakeCtrl{snap: snap, events: make(chan session.Event, 16), inst: inst}
}

func (f *fakeCtrl) record(c string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
}

func (f *fakeCtrl) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeCtrl) has(call string) bool {
	for _, c := range f.Calls() {
		if c == call {
			return true
		}
	}
	return false
}

func (f *fakeCtrl) Snapshot() (session.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap, nil
}

func (f *fakeCtrl) Events() <-chan session.Event { return f.events }
func (f *fakeCtrl) Attached() *Instance          { return f.inst }

func (f *fakeCtrl) SetActive(on bool) error {
	f.record(fmt.Sprintf("active %v", on))
	f.mu.Lock()
	f.snap.Active = on
	f.mu.Unlock()
	return nil
}

func (f *fakeCtrl) Extend(d time.Duration) error {
	f.record("extend " + d.String())
	f.mu.Lock()
	f.snap.EndsAt = f.snap.EndsAt.Add(d)
	f.mu.Unlock()
	return nil
}

// Stop ends a local fake session with a stopped event (reason user); an
// attached one only records the request.
func (f *fakeCtrl) Stop() error {
	f.record("stop")
	if f.inst == nil {
		f.end(session.ReasonUser, "stopped by user")
	}
	return nil
}

func (f *fakeCtrl) Close() error {
	f.record("close")
	return nil
}

// end publishes a stopped event and closes the stream, once.
func (f *fakeCtrl) end(r session.Reason, msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ended {
		return
	}
	f.ended = true
	snap := f.snap
	snap.Running = false
	f.events <- session.Event{Time: testNow, Type: session.EventStopped, Snapshot: snap, Reason: r, Message: msg}
	close(f.events)
}

// runningSnap is what a session started from cfg reports.
func runningSnap(cfg session.Config) session.Snapshot { return fixtureSnap(cfg) }

// starter is Options.start for tests: every session is a fakeCtrl.
type starter struct {
	mu    sync.Mutex
	ctrls []*fakeCtrl
}

func (s *starter) start(cfg session.Config) Controller {
	c := newFakeCtrl(runningSnap(cfg), nil)
	c.cfg = cfg
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctrls = append(s.ctrls, c)
	return c
}

func (s *starter) last(t *testing.T) *fakeCtrl {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ctrls) == 0 {
		t.Fatal("no session was started")
	}
	return s.ctrls[len(s.ctrls)-1]
}

func (s *starter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ctrls)
}

func testOptions(st *starter) Options {
	return Options{
		Version: "2.0.0",
		Base:    session.Config{KeepDisplay: true},
		Now:     func() time.Time { return testNow },
		Battery: func() (platform.BatteryStatus, error) {
			return platform.BatteryStatus{Percentage: 76, Available: true}, nil
		},
		ActivityProblem: func() (string, string) { return "", "" },
		LogPath:         "/home/u/.cache/keepalive/keepalive.log",
		start:           st.start,
	}
}

// harness drives a Model like Bubble Tea does: every command runs, and its
// message is fed back. Commands that block (timers, event waits) stay
// pending until settle finds their result.
type harness struct {
	t       *testing.T
	m       Model
	quit    bool
	pending []chan tea.Msg
}

func newHarness(t *testing.T, o Options, width int) *harness {
	t.Helper()
	h := &harness{t: t, m: New(o)}
	t.Cleanup(func() { _ = h.m.Shutdown() })
	h.update(tea.WindowSizeMsg{Width: width, Height: 30})
	h.exec(h.m.Init())
	return h
}

func (h *harness) update(msg tea.Msg) {
	mm, cmd := h.m.Update(msg)
	h.m = mm.(Model)
	h.exec(cmd)
}

func (h *harness) exec(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		h.deliver(msg)
	case <-time.After(30 * time.Millisecond):
		h.pending = append(h.pending, ch)
	}
}

func (h *harness) deliver(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.exec(c)
		}
	case tea.QuitMsg:
		h.quit = true
	default:
		h.update(msg)
	}
}

// settle delivers the results of pending commands that have finished.
func (h *harness) settle() {
	time.Sleep(20 * time.Millisecond)
	for progressed := true; progressed; {
		progressed = false
		for i := 0; i < len(h.pending); i++ {
			select {
			case msg := <-h.pending[i]:
				h.pending = append(h.pending[:i], h.pending[i+1:]...)
				h.deliver(msg)
				progressed = true
				i = len(h.pending) // restart: delivering may add commands
			default:
			}
		}
	}
}

func (h *harness) press(keys ...string) {
	for _, k := range keys {
		h.update(keyMsg(k))
	}
}

// typeText sends s one rune at a time, as a terminal does.
func (h *harness) typeText(s string) {
	for _, r := range s {
		h.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

var errNope = errors.New("nope")
