package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Flag groups in the help output.
const (
	groupAnnotation = "keepalive_group"
	groupSession    = "Session"
	groupActivity   = "Activity"
	groupOutput     = "Output"
)

var flagGroupOrder = []string{groupSession, groupActivity, groupOutput}

func setGroup(fs *pflag.FlagSet, group string, names ...string) {
	for _, n := range names {
		_ = fs.SetAnnotation(n, groupAnnotation, []string{group})
	}
}

func init() {
	cobra.AddTemplateFunc("groupedFlagUsages", groupedFlagUsages)
}

// groupedFlagUsages renders a command's own flags under "Session flags:",
// "Activity flags:", "Output flags:" and "Flags:" for the rest.
func groupedFlagUsages(c *cobra.Command) string {
	sets := map[string]*pflag.FlagSet{}
	other := pflag.NewFlagSet("other", pflag.ContinueOnError)
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		g := ""
		if v := f.Annotations[groupAnnotation]; len(v) > 0 {
			g = v[0]
		}
		fs := other
		if g != "" {
			if sets[g] == nil {
				sets[g] = pflag.NewFlagSet(g, pflag.ContinueOnError)
				sets[g].SortFlags = false
			}
			fs = sets[g]
		}
		fs.AddFlag(f)
	})
	var b strings.Builder
	for _, g := range flagGroupOrder {
		if fs := sets[g]; fs != nil {
			b.WriteString("\n\n" + g + " flags:\n")
			b.WriteString(strings.TrimRight(fs.FlagUsages(), " \n"))
		}
	}
	if other.HasFlags() {
		b.WriteString("\n\nFlags:\n")
		b.WriteString(strings.TrimRight(other.FlagUsages(), " \n"))
	}
	return b.String()
}

// usageTemplate is cobra's default with the flags grouped.
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{if not .AllChildCommandsHaveGroup}}

Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}{{groupedFlagUsages .}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
