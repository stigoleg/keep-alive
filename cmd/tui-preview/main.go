// Command tui-preview prints every screen of keepalive's interactive UI
// with fake data, drawn by the UI's own code, for reviewing the design in a
// real terminal. It is a developer tool: releases build only
// ./cmd/keepalive.
//
//	go run ./cmd/tui-preview [--width 64] [--height 30] [--light] [--no-color]
//	    [--ascii] [--colors truecolor|256|16] [--only dash_simulating,home]
//	go run ./cmd/tui-preview --list
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/stigoleg/keep-alive/v2/internal/tui"
)

func main() {
	width := flag.Int("width", 64, "terminal width in columns")
	height := flag.Int("height", 30, "terminal height in rows; screens drop spacing to fit")
	light := flag.Bool("light", false, "draw for a light terminal background")
	noColor := flag.Bool("no-color", false, "draw as with NO_COLOR: no colours, bold stays")
	ascii := flag.Bool("ascii", false, "draw as for a terminal that is not UTF-8")
	colors := flag.String("colors", "truecolor", "colour profile: truecolor, 256 or 16")
	only := flag.String("only", "", "comma-separated screens to show (see --list); - and _ are the same")
	list := flag.Bool("list", false, "list the screens and exit")
	flag.Parse()

	fixtures := tui.Fixtures()
	if *list {
		for _, f := range fixtures {
			fmt.Println(f.Name)
		}
		return
	}
	want := map[string]bool{}
	for _, n := range strings.Split(*only, ",") {
		if n = strings.TrimSpace(strings.ReplaceAll(n, "-", "_")); n != "" {
			want[n] = true
		}
	}
	for n := range want {
		if !hasFixture(fixtures, n) {
			fail(fmt.Errorf("no screen %q; --list shows them", n))
		}
	}

	profile, ok := map[string]termenv.Profile{"truecolor": termenv.TrueColor, "256": termenv.ANSI256, "16": termenv.ANSI}[*colors]
	if !ok {
		fail(fmt.Errorf("--colors %q: use truecolor, 256 or 16", *colors))
	}
	if *noColor {
		profile = termenv.ANSI // bold and reverse video only
	}
	// Set everything explicitly, so nothing asks the terminal.
	r := lipgloss.NewRenderer(os.Stdout)
	r.SetColorProfile(profile)
	r.SetHasDarkBackground(!*light)
	look := tui.Look{NoColor: *noColor, ASCII: *ascii}
	title := r.NewStyle().Faint(true)

	first := true
	for _, f := range fixtures {
		if len(want) > 0 && !want[f.Name] {
			continue
		}
		view := f.Build(*width, *height, r, look).View()
		if !first {
			fmt.Println()
		}
		first = false
		fmt.Println(title.Render(fmt.Sprintf("── %s · %d×%d ", f.Name, *width, strings.Count(view, "\n")+1)))
		fmt.Println(view)
	}
}

func hasFixture(fs []tui.Fixture, name string) bool {
	for _, f := range fs {
		if f.Name == name {
			return true
		}
	}
	return false
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tui-preview:", err)
	os.Exit(2)
}
