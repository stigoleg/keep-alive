package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stigoleg/keep-alive/v2/internal/config"
	"github.com/stigoleg/keep-alive/v2/internal/schedule"
	"github.com/stigoleg/keep-alive/v2/internal/session"
	"github.com/stigoleg/keep-alive/v2/internal/util"
)

type inputKind int

const (
	inputDuration inputKind = iota
	inputUntil
	inputSchedule
	inputBattery
	inputCount
)

// scheduleHint is the CLI's --schedule hint without the flag.
const scheduleHint = "example: Mon-Fri 08:00-16:00 (days Mon-Sun, weekdays, weekends or daily; 24-hour times)"

var inputs = [inputCount]struct {
	title, example string
	limit          int
}{
	inputDuration: {"Keep awake for how long?", "minutes (90) or a duration (45m, 2h30m)", 16},
	inputUntil:    {"Keep awake until what time?", "24-hour (17:00) or 12-hour (5:30pm)", 16},
	inputSchedule: {"Keep awake during which work hours?", "e.g. Mon-Fri 08:00-16:00 or weekdays 08:00-11:30,12:00-16:00", 120},
	inputBattery:  {"Stop at what battery level?", "a percentage from 1 to 100", 8},
}

// inputState keeps every input screen's text, so leaving and coming back
// finds it as it was.
type inputState struct {
	kind   inputKind
	fields [inputCount]textinput.Model
	errs   [inputCount]*Warning
}

func newInputs() inputState {
	var s inputState
	for k := range s.fields {
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = inputs[k].limit
		ti.Cursor.SetMode(cursor.CursorStatic)
		ti.Focus()
		s.fields[k] = ti
	}
	return s
}

func (m Model) openInput(k inputKind) Model {
	m.screen = screenInput
	m.input.kind = k
	m.input.errs[k] = nil
	m.home.noBattery = false
	return m
}

// inputKey handles a key on an input screen: Enter submits, Esc goes back
// keeping the text, and every other key edits the text.
func (m Model) inputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.input.errs[m.input.kind] = nil
		m.screen = screenHome
		return m, nil
	case "enter":
		return m.submit()
	}
	return m.inputUpdate(msg)
}

func (m Model) inputUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	k := m.input.kind
	before := m.input.fields[k].Value()
	var cmd tea.Cmd
	m.input.fields[k], cmd = m.input.fields[k].Update(msg)
	if m.input.fields[k].Value() != before {
		m.input.errs[k] = nil
	}
	return m, cmd
}

// submit validates the input and starts the session, or sets the battery
// level and returns to Home.
func (m Model) submit() (tea.Model, tea.Cmd) {
	k := m.input.kind
	text := strings.TrimSpace(m.input.fields[k].Value())
	h := &m.home
	if text == "" {
		text = m.lastInput(k)
	}
	if text == "" {
		m.input.errs[k] = &Warning{Text: "type a value first", Fix: inputs[k].example}
		return m, nil
	}
	cfg := m.sessionConfig()
	switch k {
	case inputDuration:
		d, err := config.ParseDuration(text)
		if err != nil {
			m.input.errs[k] = &Warning{Text: err.Error()}
			return m, nil
		}
		h.lastDuration, cfg.Duration = d, d
	case inputUntil:
		t, err := util.NextClockTime(text, m.now())
		if err != nil {
			m.input.errs[k] = &Warning{Text: err.Error()}
			return m, nil
		}
		h.lastUntil, cfg.Until = t.Format("15:04"), t
	case inputSchedule:
		s, err := schedule.Parse(text)
		if err != nil {
			m.input.errs[k] = &Warning{Text: strings.TrimPrefix(err.Error(), "schedule: "), Fix: scheduleHint}
			return m, nil
		}
		h.lastSchedule, cfg.Schedule = s.String(), s
	case inputBattery:
		n, w := m.checkBattery(text)
		if w != nil {
			m.input.errs[k] = w
			return m, nil
		}
		h.opts.battery, h.lastBattery = n, n
		m.input.fields[k].Reset()
		m.screen = screenHome
		return m, nil
	}
	m.input.fields[k].Reset()
	return m.startSession(cfg)
}

// checkBattery validates a threshold as the CLI does for --battery.
func (m Model) checkBattery(text string) (int, *Warning) {
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, &Warning{Text: fmt.Sprintf("invalid battery percentage %q", text), Fix: "type a whole number, e.g. 20"}
	}
	if n < 1 || n > 100 {
		return 0, &Warning{Text: fmt.Sprintf("battery threshold must be between 1 and 100 (got %d)", n)}
	}
	st, err := m.battery()
	if err != nil || !st.Available {
		if err == nil {
			err = fmt.Errorf("not available")
		}
		return 0, &Warning{
			Text: fmt.Sprintf("battery threshold %d%% set, but no battery was found (%v)", n, err),
			Fix:  `"Stop at battery" only works on machines with a battery`,
		}
	}
	if st.Percentage <= n {
		return 0, &Warning{
			Text: fmt.Sprintf("battery threshold must be below the current level (current %d%%, threshold %d%%)", st.Percentage, n),
			Fix:  "choose a lower value",
		}
	}
	return n, nil
}

// lastInput is what Enter on an empty input uses.
func (m Model) lastInput(k inputKind) string {
	h := m.home
	switch k {
	case inputDuration:
		return shortDurationOr(h.lastDuration)
	case inputUntil:
		return h.lastUntil
	case inputSchedule:
		return h.lastSchedule
	case inputBattery:
		if h.lastBattery > 0 {
			return strconv.Itoa(h.lastBattery)
		}
	}
	return ""
}

func (m Model) inputView() string {
	return m.fit(func(level int) string { return m.inputPage(level).String() })
}

func (m Model) inputPage(level int) *canvas {
	st, k := m.st, m.input.kind
	c := newCanvas(st, m.width, level, true)
	c.header(st.Muted.Render(versionText(m.version)))
	c.spacer()
	c.add(st.label(inputs[k].title, 0))
	field := m.input.fields[k]
	c.add(st.Accent.Render(st.g.prompt) + " " + m.inputLine(field, c.width-2))
	example := inputs[k].example
	if k == inputBattery && m.home.battery.read && m.home.battery.status.Available {
		example += fmt.Sprintf("; the battery is at %d%% now", m.home.battery.status.Percentage)
	}
	c.wrapped("  "+example, "  ", st.Muted)
	if last := m.lastInput(k); last != "" && field.Value() == "" {
		c.add(st.Muted.Render("  enter alone uses " + last))
	}

	if w := m.input.errs[k]; w != nil {
		c.spacer()
		c.mark(st.Problem, st.g.invalid, w.Text, st.Plain)
		if w.Fix != "" {
			c.wrapped("  "+w.Fix, "  ", st.Muted)
		}
	} else if ok, more := m.preview(k, strings.TrimSpace(field.Value())); ok != "" {
		c.spacer()
		ok, more = st.text(ok), st.text(more)
		if one := ok + " " + st.g.sep + " " + more; more != "" && lipgloss.Width(one)+2 <= c.width {
			c.add(st.OK.Render(st.g.valid) + " " + ok + st.Muted.Render(" "+st.g.sep+" "+more))
		} else {
			c.mark(st.OK, st.g.valid, ok, st.Plain)
			if more != "" {
				c.wrapped("  "+more, "  ", st.Muted)
			}
		}
	}
	verb := "start"
	if k == inputBattery {
		verb = "set"
	}
	c.keyHints(keyHint{"⏎", verb, ""}, keyHint{"esc", "back", ""}, keyHint{"ctrl+c", "quit", ""})
	return c
}

// preview describes a valid value as it is typed: what it means, and more
// detail that may go on the same line.
func (m Model) preview(k inputKind, text string) (ok, more string) {
	if text == "" {
		return "", ""
	}
	now := m.now()
	switch k {
	case inputDuration:
		d, err := config.ParseDuration(text)
		if err != nil {
			return "", ""
		}
		return shortDuration(d), "until " + session.ClockText(now, now.Add(d))
	case inputUntil:
		t, err := util.NextClockTime(text, now)
		if err != nil {
			return "", ""
		}
		day := "today"
		if y, mo, d := t.Date(); y != now.Year() || mo != now.Month() || d != now.Day() {
			day = "tomorrow"
		}
		return fmt.Sprintf("until %s %s", t.Format("15:04"), day), span(t.Sub(now)) + " from now"
	case inputSchedule:
		s, err := schedule.Parse(text)
		if err != nil {
			return "", ""
		}
		return m.st.scheduleText(s.String()), scheduleNext(s, now)
	}
	return "", ""
}

// scheduleNext is "now: until 16:00" inside a window, else
// "next: Mon 08:00–16:00".
func scheduleNext(s *schedule.Schedule, now time.Time) string {
	_, end, in := s.Current(now)
	switch {
	case in && end.IsZero():
		return "now: always (the whole week)"
	case in:
		return "now: until " + session.ClockText(now, end)
	}
	next, ok := s.Next(now)
	if !ok {
		return ""
	}
	_, end, _ = s.Current(next)
	endText := end.Format("15:04")
	if y, mo, d := end.Date(); y != next.Year() || mo != next.Month() || d != next.Day() {
		endText = end.Format("Mon 15:04")
	}
	return fmt.Sprintf("next: %s–%s", next.Format("Mon 15:04"), endText)
}

// inputLine renders the text with the cursor, scrolled so the cursor stays
// visible within width.
func (m Model) inputLine(ti textinput.Model, width int) string {
	val := []rune(ti.Value())
	pos := min(ti.Position(), len(val))
	width = max(width-1, 4)
	from := 0
	if pos >= width {
		from = pos - width + 1
	}
	to := min(len(val), from+width)
	before, after := string(val[from:pos]), ""
	under := " "
	if pos < to {
		under, after = string(val[pos]), string(val[pos+1:to])
	}
	if m.st.plain {
		if under != " " {
			after = under + after
		}
		return before + "█" + after
	}
	return before + m.st.Cursor.Render(under) + after
}
