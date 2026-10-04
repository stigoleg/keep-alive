// Command gen-docs writes the man pages and shell completions from the cobra
// command tree:
//
//	man/keepalive.1 (plus one page per subcommand)
//	docs/completions/keepalive.bash, _keepalive, keepalive.fish, keepalive.ps1
//
// GoReleaser runs it as a before hook and packages these paths.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/stigoleg/keep-alive/v2/internal/cli"
)

func main() {
	root := cli.NewRootCommand("")
	root.DisableAutoGenTag = true
	if err := writeMan(root, "man"); err != nil {
		fail(err)
	}
	if err := writeCompletions(root, filepath.Join("docs", "completions")); err != nil {
		fail(err)
	}
}

func writeMan(root *cobra.Command, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	header := &doc.GenManHeader{Title: "KEEPALIVE", Section: "1", Source: "keep-alive", Manual: "User Commands"}
	return doc.GenManTree(root, header, dir)
}

func writeCompletions(root *cobra.Command, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	gens := []struct {
		name string
		gen  func(path string) error
	}{
		{"keepalive.bash", func(p string) error { return root.GenBashCompletionFileV2(p, true) }},
		{"_keepalive", root.GenZshCompletionFile},
		{"keepalive.fish", func(p string) error { return root.GenFishCompletionFile(p, true) }},
		{"keepalive.ps1", root.GenPowerShellCompletionFileWithDesc},
	}
	for _, g := range gens {
		if err := g.gen(filepath.Join(dir, g.name)); err != nil {
			return fmt.Errorf("%s: %w", g.name, err)
		}
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-docs:", err)
	os.Exit(1)
}
