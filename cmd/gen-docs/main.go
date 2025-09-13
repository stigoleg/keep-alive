package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This small tool generates shell completions and a man page based on the known flags.
// It does not depend on Cobra; it emits simple, robust completions for common shells
// and a minimal roff man page that mirrors --help contents.

const (
	appName        = "keepalive"
	appDescription = "A lightweight, cross-platform utility to prevent your system from going to sleep."
)

type flagDef struct {
	Short string
	Long  string
	Arg   string
	Desc  string
}

func main() {
	flags := []flagDef{
		{Short: "-d", Long: "--duration", Arg: "<string>", Desc: "Duration to keep system alive (e.g., \"2h30m\" or \"150\")"},
		{Short: "-c", Long: "--clock", Arg: "<string>", Desc: "Time to keep system alive until (e.g., \"22:00\" or \"10:00PM\")"},
		{Short: "-v", Long: "--version", Arg: "", Desc: "Show version information"},
		{Short: "-h", Long: "--help", Arg: "", Desc: "Show help message"},
	}

	if err := writeCompletions(flags); err != nil {
		panic(err)
	}
	if err := writeMan(flags); err != nil {
		panic(err)
	}
}

func writeCompletions(flags []flagDef) error {
	base := filepath.Join("docs", "completions")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}

	// Bash
	var bash strings.Builder
	bash.WriteString("_" + appName + "() {\n")
	bash.WriteString("  local cur prev opts\n")
	bash.WriteString("  COMPREPLY=()\n")
	bash.WriteString("  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	var opts []string
	for _, f := range flags {
		if f.Short != "" {
			opts = append(opts, f.Short)
		}
		if f.Long != "" {
			opts = append(opts, f.Long)
		}
	}
	bash.WriteString("  opts=\"" + strings.Join(opts, " ") + "\"\n")
	bash.WriteString("  if [[ ${cur} == -* ]] ; then\n")
	bash.WriteString("    COMPREPLY=( $(compgen -W \"${opts}\" -- ${cur}) )\n")
	bash.WriteString("    return 0\n")
	bash.WriteString("  fi\n")
	bash.WriteString("}\n")
	bash.WriteString("complete -F _" + appName + " " + appName + "\n")
	if err := os.WriteFile(filepath.Join(base, appName+".bash"), []byte(bash.String()), 0o644); err != nil {
		return err
	}

	// Zsh
	var zsh strings.Builder
	zsh.WriteString("#compdef " + appName + "\n")
	zsh.WriteString("_arguments ")
	var parts []string
	for _, f := range flags {
		form := fmt.Sprintf("'%s[%s]%s'", zFlagName(f), f.Desc, zArgSuffix(f.Arg))
		parts = append(parts, form)
	}
	zsh.WriteString(strings.Join(parts, " ") + "\n")
	if err := os.WriteFile(filepath.Join(base, "_"+appName), []byte(zsh.String()), 0o644); err != nil {
		return err
	}

	// Fish
	var fish strings.Builder
	fish.WriteString("complete -c " + appName + " -f\n")
	for _, f := range flags {
		fish.WriteString(fishFlagLine(f))
	}
	if err := os.WriteFile(filepath.Join(base, appName+".fish"), []byte(fish.String()), 0o644); err != nil {
		return err
	}

	return nil
}

func zFlagName(f flagDef) string {
	if f.Arg != "" {
		// zsh requires = for options with arguments
		if f.Long != "" {
			return f.Long + "="
		}
		return f.Short + "="
	}
	if f.Long != "" {
		return f.Long
	}
	return f.Short
}

func zArgSuffix(arg string) string {
	if arg == "" {
		return ""
	}
	return ":value:" + strings.Trim(arg, "<>")
}

func fishFlagLine(f flagDef) string {
	var b strings.Builder
	b.WriteString("complete -c ")
	b.WriteString(appName)
	if f.Short != "" {
		b.WriteString(" -s ")
		b.WriteString(strings.TrimPrefix(f.Short, "-"))
	}
	if f.Long != "" {
		b.WriteString(" -l ")
		b.WriteString(strings.TrimPrefix(f.Long, "--"))
	}
	if f.Arg != "" {
		b.WriteString(" -r")
	} else {
		b.WriteString(" -f")
	}
	b.WriteString(" -d \"")
	b.WriteString(escapeDoubleQuotes(f.Desc))
	b.WriteString("\"\n")
	return b.String()
}

func escapeDoubleQuotes(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}

func writeMan(flags []flagDef) error {
	if err := os.MkdirAll("man", 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(".TH \"" + strings.ToUpper(appName) + "\" \"1\" \"\" \"keep-alive\" \"User Commands\"\n")
	b.WriteString(".SH NAME\n" + appName + " - " + appDescription + "\n")
	b.WriteString(".SH SYNOPSIS\n.B " + appName + "\n")
	b.WriteString("[\\-d|\\-\\-duration <string>] [\\-c|\\-\\-clock <string>] [\\-v|\\-\\-version] [\\-h|\\-\\-help]\\n")
	b.WriteString(".SH DESCRIPTION\n" + appDescription + "\n")
	b.WriteString(".SH OPTIONS\n")
	for _, f := range flags {
		names := f.Short
		if f.Long != "" {
			if names != "" {
				names += ", "
			}
			names += f.Long
		}
		if f.Arg != "" {
			names += " " + f.Arg
		}
		b.WriteString(".TP\n\fB" + names + "\fR\n" + f.Desc + "\n")
	}
	b.WriteString(".SH EXAMPLES\n")
	b.WriteString(".TP\n\fB" + appName + "\fR\nStart interactive TUI.\n")
	b.WriteString(".TP\n\fB" + appName + " -d 2h30m\fR\nKeep system awake for 2 hours 30 minutes.\n")
	b.WriteString(".TP\n\fB" + appName + " -c 22:00\fR\nKeep system awake until 10:00 PM.\n")
	b.WriteString(".SH SEE ALSO\nProject homepage: https://github.com/stigoleg/keep-alive\n")
	return os.WriteFile(filepath.Join("man", appName+".1"), []byte(b.String()), 0o644)
}
