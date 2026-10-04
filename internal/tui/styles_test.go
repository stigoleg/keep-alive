package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

func TestNoColorRendersPlainText(t *testing.T) {
	st := NewStyles(NewRenderer(os.Stdout, false))
	if got := st.Problem.Render("x") + st.Cursor.Render("y"); got != "xy" {
		t.Fatalf("styled without colour: %q", got)
	}
	if !st.plain {
		t.Fatal("plain not set")
	}
}

func TestRendererKeepsTheCachedBackground(t *testing.T) {
	r := NewRenderer(os.Stdout, true)
	if r.HasDarkBackground() != lipgloss.HasDarkBackground() {
		t.Fatal("background differs from the cached answer")
	}
}
