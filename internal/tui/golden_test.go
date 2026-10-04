package tui

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var update = flag.Bool("update", false, "rewrite golden files")

// goldenWidths are the terminal widths every fixture is checked at: the
// target, the narrowest with a frame, and the narrowest supported.
var goldenWidths = []int{64, 48, 40}

// goldenHeight is tall enough for every screen.
const goldenHeight = 30

// ansiGoldens are drawn in truecolor on a dark background too, so colour
// changes show in diffs.
var ansiGoldens = []string{"home", "dash_simulating", "dash_problem", "input_schedule_preview"}

// TestGolden compares every fixture at every width with
// testdata/<name>_<width>.golden (plain text), and a few in colour with
// testdata/<name>_<width>.ansi.golden; no line may be wider than the
// terminal.
func TestGolden(t *testing.T) {
	for _, f := range Fixtures() {
		for _, w := range goldenWidths {
			checkGolden(t, fmt.Sprintf("%s_%d.golden", f.Name, w), f.Build(w, goldenHeight, nil, Look{}).View(), w)
		}
		if slices.Contains(ansiGoldens, f.Name) {
			view := f.Build(64, goldenHeight, forced(termenv.TrueColor, true), Look{}).View()
			checkGolden(t, fmt.Sprintf("%s_64.ansi.golden", f.Name), view, 64)
		}
	}
}

func checkGolden(t *testing.T, file, view string, width int) {
	t.Helper()
	for i, l := range strings.Split(view, "\n") {
		if lipgloss.Width(l) > width {
			t.Errorf("%s: line %d is %d wide: %q", file, i+1, lipgloss.Width(l), l)
		}
	}
	got := []byte(view + "\n")
	path := filepath.Join("testdata", file)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s:\n got:\n%s\nwant:\n%s", file, got, want)
	}
}

// TestGoldenShortTerminals draws screens in terminals too short for them:
// spacing, the HOLDING row, the sparkline, the callout's note and the big
// digits go, in that order.
func TestGoldenShortTerminals(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"dash_simulating", 64, 14},
		{"dash_simulating", 40, 11},
		{"dash_problem", 64, 17},
		{"dash_problem", 40, 20},
		{"home_warning", 40, 20},
	} {
		view := fixtureNamed(t, tc.name).Build(tc.width, tc.height, nil, Look{}).View()
		checkGolden(t, fmt.Sprintf("%s_%dx%d.golden", tc.name, tc.width, tc.height), view, tc.width)
		if n := strings.Count(view, "\n") + 1; n > tc.height {
			t.Errorf("%s at %dx%d: %d lines", tc.name, tc.width, tc.height, n)
		}
	}
}

func fixtureNamed(t *testing.T, name string) Fixture {
	t.Helper()
	for _, f := range Fixtures() {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no fixture %s", name)
	return Fixture{}
}

// TestShortTerminalDropOrder follows the problem dashboard as the
// terminal gets shorter.
func TestShortTerminalDropOrder(t *testing.T) {
	f := fixtureNamed(t, "dash_problem")
	full := f.Build(64, 40, nil, Look{}).View()
	has := func(view, s string) bool { return strings.Contains(view, s) }
	lines := func(view string) int { return strings.Count(view, "\n") + 1 }
	if !has(full, "HOLDING") || !has(full, "checks again every 60 s") || !has(full, "▀▀▀") || !has(full, "│                                                              │") {
		t.Fatalf("full view:\n%s", full)
	}
	prev := lines(full)
	var seen []string
	for h := prev - 1; h >= 8; h-- {
		v := f.Build(64, h, nil, Look{}).View()
		if lines(v) > prev {
			t.Fatalf("height %d: taller than at %d", h, h+1)
		}
		prev = lines(v)
		for _, step := range []struct{ name, gone string }{
			{"spacers", "│                                                              │"},
			{"holding", "HOLDING"},
			{"note", "checks again every 60 s"},
			{"digits", "▀▀▀"},
		} {
			if !has(v, step.gone) && !slices.Contains(seen, step.name) {
				seen = append(seen, step.name)
			}
		}
	}
	if want := []string{"spacers", "holding", "note", "digits"}; !slices.Equal(seen, want) {
		t.Fatalf("dropped %v, want %v", seen, want)
	}
	if v := f.Build(64, 8, nil, Look{}).View(); !has(v, "48m running") {
		t.Fatalf("no one-line time:\n%s", v)
	}
	// The sparkline goes before the note, and the state text stays.
	s := fixtureNamed(t, "dash_simulating")
	for h := 20; h >= 8; h-- {
		v := s.Build(64, h, nil, Look{}).View()
		if !has(v, "◉ simulating") {
			t.Fatalf("height %d: state text gone:\n%s", h, v)
		}
		if !has(v, "▁") && has(v, "▀▀▀") && has(v, "HOLDING") {
			t.Fatalf("height %d: sparkline gone before HOLDING:\n%s", h, v)
		}
	}
}

// TestFixturesFitEveryWidth draws every fixture in every look at every
// width from 30 to 100 columns: no line may be wider than the terminal.
func TestFixturesFitEveryWidth(t *testing.T) {
	looks := []struct {
		r    *lipgloss.Renderer
		look Look
	}{
		{nil, Look{}},
		{forced(termenv.TrueColor, true), Look{}},
		{forced(termenv.ANSI, false), Look{NoColor: true}},
		{nil, Look{ASCII: true}},
	}
	for _, f := range Fixtures() {
		for _, l := range looks {
			for w := 30; w <= 100; w++ {
				for i, line := range strings.Split(f.Build(w, goldenHeight, l.r, l.look).View(), "\n") {
					if lipgloss.Width(line) > w {
						t.Fatalf("%s at %d (%+v): line %d is %d wide: %q", f.Name, w, l.look, i+1, lipgloss.Width(line), line)
					}
				}
			}
		}
	}
}

func TestFixtureNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Fixtures() {
		if seen[f.Name] {
			t.Fatalf("two fixtures named %s", f.Name)
		}
		seen[f.Name] = true
	}
	for _, n := range ansiGoldens {
		if !seen[n] {
			t.Fatalf("ansi golden %s has no fixture", n)
		}
	}
}

func tickMsgFor(h *harness) tickMsg { return tickMsg{gen: h.m.tick} }
