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
