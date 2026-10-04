package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	colorful "github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"
)

func TestGradientEndpointsAreExact(t *testing.T) {
	for _, stops := range [][]gradientStop{gradDark, gradLight, sparkDark, sparkLight} {
		for _, n := range []int{2, 3, 9, 60} {
			g := gradient(n, stops)
			if len(g) != n {
				t.Fatalf("%d colours, want %d", len(g), n)
			}
			if g[0] != strings.ToLower(stops[0].hex) || g[n-1] != strings.ToLower(stops[len(stops)-1].hex) {
				t.Errorf("n=%d: ends %s…%s, want %s…%s", n, g[0], g[n-1], stops[0].hex, stops[len(stops)-1].hex)
			}
		}
	}
	// A stop in the middle is hit exactly where it sits: 0.55 = 11/20.
	if g := gradient(21, gradDark); g[11] != "#f472b6" {
		t.Fatalf("middle stop %s", g[11])
	}
}

func TestGradientBlendIsMonotonic(t *testing.T) {
	stops := []gradientStop{{"#A78BFA", 0}, {"#FBBF24", 1}}
	start, _ := colorful.Hex(stops[0].hex)
	end, _ := colorful.Hex(stops[1].hex)
	prevFrom, prevTo := -1.0, 2.0
	for _, hex := range gradient(30, stops) {
		c, err := colorful.Hex(hex)
		if err != nil {
			t.Fatal(err)
		}
		from, to := distOkLab(start, c), distOkLab(c, end)
		if from < prevFrom-1e-3 || to > prevTo+1e-3 {
			t.Fatalf("%s goes backwards: %.4f after %.4f from the start", hex, from, prevFrom)
		}
		prevFrom, prevTo = from, to
	}
}

func distOkLab(a, b colorful.Color) float64 {
	l1, a1, b1 := a.OkLab()
	l2, a2, b2 := b.OkLab()
	return (l1-l2)*(l1-l2) + (a1-a2)*(a1-a2) + (b1-b2)*(b1-b2)
}

func TestGradientSmallSizes(t *testing.T) {
	if g := gradient(1, gradDark); len(g) != 1 || g[0] != "#a78bfa" {
		t.Fatalf("one colour: %v", g)
	}
	if g := gradient(0, gradDark); g != nil {
		t.Fatalf("no colours: %v", g)
	}
	if g := gradient(-1, gradDark); g != nil {
		t.Fatalf("negative: %v", g)
	}
	st := NewStyles(forced(termenv.TrueColor, true), Look{})
	if got := ansi.Strip(st.paint([]string{"x"}, gradDark, true)); got != "x" {
		t.Fatalf("paint one cell: %q", got)
	}
	if got := st.paint(nil, gradDark, false); ansi.Strip(got) != "" {
		t.Fatalf("paint nothing: %q", got)
	}
}

func TestKeyLinesFitTheWidth(t *testing.T) {
	keys := []keyHint{
		{"a", "activity off", "activity"}, {"+/-", "15 min", ""}, {"s", "stop", ""}, {"?", "help", ""}, {"q", "quit", ""},
	}
	for _, look := range []Look{{}, {NoColor: true}, {ASCII: true}} {
		for _, r := range []*lipgloss.Renderer{nil, forced(termenv.TrueColor, true)} {
			st := NewStyles(r, look)
			for w := 10; w <= 64; w++ {
				lines := st.keyLines(w, keys)
				for _, l := range lines {
					if lipgloss.Width(l) > w {
						t.Fatalf("%+v at %d: %q is %d wide", look, w, ansi.Strip(l), lipgloss.Width(l))
					}
				}
				if w >= 40 && len(lines) > 2 {
					t.Errorf("%+v at %d: %d lines", look, w, len(lines))
				}
			}
		}
	}
	st := NewStyles(nil, Look{})
	if got := st.keyLines(64, keys)[0]; got != "[a] activity off  [+/-] 15 min  [s] stop  [?] help  [q] quit" {
		t.Fatalf("full labels: %q", got)
	}
	if got := st.keyLines(56, keys)[0]; got != "[a] activity  [+/-] 15 min  [s] stop  [?] help  [q] quit" {
		t.Fatalf("short labels: %q", got)
	}
	col := NewStyles(forced(termenv.TrueColor, true), Look{})
	if got := ansi.Strip(col.keyLines(64, keys)[0]); got != " a  activity off   +/-  15 min   s  stop   ?  help   q  quit" {
		t.Fatalf("keycaps: %q", got)
	}
}

func TestBoxAndCallout(t *testing.T) {
	st := NewStyles(nil, Look{})
	got := st.callout(st.Problem, 30, calloutText{
		text: "Accessibility is off for Ghostty, so keepalive can't move the pointer.",
		fix:  "System Settings → Privacy & Security → Accessibility",
		note: "keepalive checks again every 60 s",
	}, true, true)
	want := []string{
		"╭────────────────────────────╮",
		"│ Accessibility is off for   │",
		"│ Ghostty, so keepalive      │",
		"│ can't move the pointer.    │",
		"│                            │",
		"│ Fix  System Settings →     │",
		"│      Privacy & Security →  │",
		"│      Accessibility         │",
		"│ keepalive checks again     │",
		"│ every 60 s                 │",
		"╰────────────────────────────╯",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("callout:\n%s", strings.Join(got, "\n"))
	}
	short := st.callout(st.Problem, 30, calloutText{text: "x", fix: "y", note: "z"}, false, false)
	if len(short) != 4 {
		t.Fatalf("short callout:\n%s", strings.Join(short, "\n"))
	}
	for _, l := range st.box(st.Frame, 12, []string{"far too long for this box"}) {
		if lipgloss.Width(l) != 12 {
			t.Fatalf("box line %q is %d wide", l, lipgloss.Width(l))
		}
	}
}

func TestBars(t *testing.T) {
	st := NewStyles(nil, Look{})
	for _, tc := range []struct{ got, want string }{
		{st.blockBar(0.5, 10), "█████░░░░░"},
		{st.blockBar(-1, 4), "░░░░"},
		{st.blockBar(2, 4), "████"},
		{st.lineBar(0.25, 8), "━━──────"},
		{st.meter(0.76, 20, st.OK), "━━━━━━━━━━━━━━━─────"},
	} {
		if tc.got != tc.want {
			t.Errorf("bar %q, want %q", tc.got, tc.want)
		}
	}
	col := NewStyles(forced(termenv.TrueColor, true), Look{})
	if got := ansi.Strip(col.blockBar(0.5, 10)); got != "██████████" {
		t.Fatalf("colour bar %q", got)
	}
	if got := ansi.Strip(col.meter(0.5, 4, col.OK)); got != "━━━━" {
		t.Fatalf("colour meter %q", got)
	}
}

func TestSparklineLevels(t *testing.T) {
	st := NewStyles(nil, Look{})
	for _, tc := range []struct {
		counts []int
		want   string
	}{
		{[]int{0, 0, 0}, "▁▁▁"},
		{[]int{0, 1, 0}, "▁▅▁"},      // a lone burst stands half high
		{[]int{1, 2, 0}, "▅█▁"},      // scaled to the largest
		{[]int{1, 7, 14, 3}, "▂▅█▃"}, // ceil(c*7/14)
		{[]int{0, 1, 2, 5}, "▁▃▄█"},  // ceil(1*7/5)=2, ceil(2*7/5)=3
		{[]int{}, ""},
	} {
		if got := st.sparkline(tc.counts); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.counts, got, tc.want)
		}
	}
}

func TestBigText(t *testing.T) {
	st := NewStyles(nil, Look{})
	got := st.bigText("1:12")
	want := [3]string{
		"▀█    ▀█  ▀▀█",
		" █  ▀  █  █▀▀",
		"▀▀▀ ▀ ▀▀▀ ▀▀▀",
	}
	if got != want {
		t.Fatalf("big text:\n%s", strings.Join(got[:], "\n"))
	}
	for _, d := range "0123456789" {
		for _, row := range st.bigText(string(d)) {
			if lipgloss.Width(row) != 3 {
				t.Fatalf("%c is %d wide", d, lipgloss.Width(row))
			}
		}
	}
}

func TestBandAndPills(t *testing.T) {
	col := NewStyles(forced(termenv.TrueColor, true), Look{})
	b := col.band(30, []seg{{"❯ Until I stop it", col.Selected}}, []seg{{"2h", col.Muted}})
	if lipgloss.Width(b) != 30 || ansi.Strip(b) != "❯ Until I stop it           2h" {
		t.Fatalf("band %q", ansi.Strip(b))
	}
	// Every cell sits on the band colour, so no reset may leave a gap.
	for _, part := range strings.Split(b, "\x1b[0m") {
		if part != "" && !strings.Contains(part, "48;2;") {
			t.Fatalf("a part of the band has no background: %q", part)
		}
	}
	for _, tc := range []struct {
		k          pillKind
		sym, text  string
		want, mono string
	}{
		{pillOK, "●", "AWAKE", " ● AWAKE ", "[AWAKE]"},
		{pillWarn, "◐", "PAUSED", " ◐ PAUSED ", "[PAUSED]"},
		{pillBad, "▲", "ACTION NEEDED", " ▲ ACTION NEEDED ", "[ACTION NEEDED]"},
		{pillMuted, "■", "STOPPED", " ■ STOPPED ", "[STOPPED]"},
	} {
		if got := ansi.Strip(col.pill(tc.k, tc.sym, tc.text)); got != tc.want {
			t.Errorf("pill %q, want %q", got, tc.want)
		}
		if got := NewStyles(nil, Look{}).pill(tc.k, tc.sym, tc.text); got != tc.mono {
			t.Errorf("plain pill %q, want %q", got, tc.mono)
		}
	}
	if got := ansi.Strip(col.wordmark()); got != "keepalive" {
		t.Fatalf("wordmark %q", got)
	}
	if versionText("2.0.0") != "v2.0.0" || versionText("dev") != "dev" || versionText("") != "" {
		t.Fatal("versionText")
	}
}
