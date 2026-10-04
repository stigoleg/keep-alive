package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// TestNoStylingAtInit makes sure importing the package does no terminal
// work: no package-level variable is initialized with a lipgloss or termenv
// call, and there is no init function. Styles are built by New.
func TestNoStylingAtInit(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" {
					t.Errorf("%s: init function", fset.Position(d.Pos()))
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					for _, v := range spec.(*ast.ValueSpec).Values {
						ast.Inspect(v, func(n ast.Node) bool {
							call, ok := n.(*ast.CallExpr)
							if !ok {
								return true
							}
							if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
								if pkg, ok := sel.X.(*ast.Ident); ok && (pkg.Name == "lipgloss" || pkg.Name == "termenv") {
									t.Errorf("%s: package-level %s.%s call", fset.Position(call.Pos()), pkg.Name, sel.Sel.Name)
								}
							}
							return true
						})
					}
				}
			}
		}
	}
}

func TestPlainRendererDrawsText(t *testing.T) {
	st := NewStyles(NewRenderer(io.Discard, false), Look{})
	if got := st.Problem.Render("x") + st.Cursor.Render("y") + st.Bold.Render("z") + st.pill(pillOK, "●", "AWAKE"); got != "xyz[AWAKE]" {
		t.Fatalf("styled without a terminal: %q", got)
	}
	if !st.plain || st.color {
		t.Fatal("plain not set")
	}
}

// forced is a renderer with profile p on a dark or light background, for
// tests that look at escape sequences.
func forced(p termenv.Profile, dark bool) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(p)
	r.SetHasDarkBackground(dark)
	return r
}

var sgr = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// hasColour reports whether s sets a foreground or background colour.
func hasColour(s string) bool {
	for _, m := range sgr.FindAllStringSubmatch(s, -1) {
		for _, p := range strings.Split(m[1], ";") {
			n, _ := strconv.Atoi(p)
			if p == "38" || p == "48" || (n >= 30 && n <= 37) || (n >= 40 && n <= 47) || (n >= 90 && n <= 97) || (n >= 100 && n <= 107) {
				return true
			}
		}
	}
	return false
}

func TestNoColorKeepsBoldButNoColour(t *testing.T) {
	st := NewStyles(forced(termenv.ANSI, true), Look{NoColor: true})
	out := st.wordmark() + st.Problem.Render("x") + st.pill(pillBad, "▲", "ACTION NEEDED") + st.keycap("a") +
		st.blockBar(0.5, 10) + st.sparkline([]int{0, 1, 2}) + st.band(20, []seg{{"❯ Until", st.Selected}}, nil)
	if hasColour(out) {
		t.Fatalf("colour with NO_COLOR: %q", out)
	}
	if !strings.Contains(out, "\x1b[1m") {
		t.Fatalf("no bold with NO_COLOR: %q", out)
	}
	if got, want := ansi.Strip(out), "keepalivex[ACTION NEEDED][a]█████░░░░░▁▅█❯ Until             "; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := ansi.Strip(st.pill(pillOK, "●", "AWAKE") + st.keycap("q")); got != "[AWAKE][q]" {
		t.Fatalf("pill and keycap without colour: %q", got)
	}
}

func TestColourProfiles(t *testing.T) {
	for _, p := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.ANSI} {
		st := NewStyles(forced(p, true), Look{})
		if !hasColour(st.pill(pillOK, "●", "AWAKE")) || !hasColour(st.wordmark()) {
			t.Errorf("profile %v draws no colour", p)
		}
	}
	dark := NewStyles(forced(termenv.TrueColor, true), Look{}).OK.Render("x")
	light := NewStyles(forced(termenv.TrueColor, false), Look{}).OK.Render("x")
	if dark == light {
		t.Fatal("light and dark backgrounds use the same colour")
	}
	if !strings.Contains(dark, "38;2;") || !strings.Contains(light, "38;2;") {
		t.Fatalf("ok colour: dark %q light %q", dark, light)
	}
	// 256 and 16 colours use the hand-picked values: grey, not navy.
	if got := NewStyles(forced(termenv.ANSI256, true), Look{}).Band.Render("x"); !strings.Contains(got, "48;5;235") {
		t.Fatalf("256-colour band %q", got)
	}
	if got := NewStyles(forced(termenv.ANSI, true), Look{}).Key.Render("x"); !strings.Contains(got, "100") {
		t.Fatalf("16-colour keycap %q", got)
	}
}

func TestUnicodeTerminal(t *testing.T) {
	env := func(kv ...string) func(string) (string, bool) {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	for _, tc := range []struct {
		name string
		env  func(string) (string, bool)
		goos string
		want bool
	}{
		{"LANG utf-8", env("LANG", "en_US.UTF-8"), "linux", true},
		{"LANG utf8", env("LANG", "C.utf8"), "linux", true},
		{"LANG latin1", env("LANG", "de_DE.ISO-8859-1"), "linux", false},
		{"LC_ALL wins", env("LC_ALL", "C", "LANG", "en_US.UTF-8"), "linux", false},
		{"LC_CTYPE before LANG", env("LC_CTYPE", "UTF-8", "LANG", "C"), "linux", true},
		{"empty LC_ALL is skipped", env("LC_ALL", "", "LANG", "en_US.UTF-8"), "linux", true},
		{"no locale on Linux", env(), "linux", false},
		{"no locale on macOS", env(), "darwin", true},
		{"C locale on macOS", env("LANG", "C"), "darwin", false},
		{"Windows Terminal", env("WT_SESSION", "x"), "windows", true},
		{"Windows console", env(), "windows", false},
		{"Git Bash", env("LANG", "en_US.UTF-8"), "windows", true},
	} {
		if got := UnicodeTerminal(tc.env, tc.goos); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestASCIILookDrawsASCII(t *testing.T) {
	st := NewStyles(nil, Look{ASCII: true})
	big := st.bigText("1:2")
	out := strings.Join(st.box(st.Frame, 40, []string{st.text("For a duration… → Mon–Fri · ✓")}), "\n") +
		st.blockBar(0.5, 10) + st.lineBar(0.5, 10) + st.meter(0.5, 10, st.OK) + st.sparkline([]int{0, 1, 2, 5}) +
		strings.Join(big[:], "") + strings.Join(st.keyLines(30, []keyHint{{"↑↓", "move", ""}}), "")
	for _, r := range out {
		if r > 127 {
			t.Fatalf("non-ASCII %q in %q", r, out)
		}
	}
	if !strings.Contains(out, "| For a duration... -> Mon-Fri - +     |") || !strings.Contains(out, "[^v] move") {
		t.Fatalf("text not converted: %q", out)
	}
}

func TestRendererKeepsTheCachedBackground(t *testing.T) {
	r := NewRenderer(os.Stdout, true)
	if r.HasDarkBackground() != lipgloss.HasDarkBackground() {
		t.Fatal("background differs from the cached answer")
	}
}
