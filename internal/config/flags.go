package config

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/stigoleg/keep-alive/internal/ui"
	"github.com/stigoleg/keep-alive/internal/util"
)

type Config struct {
	Duration    int
	ShowVersion bool
}

func formatError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "Invalid duration format:") {
		parts := strings.SplitN(msg, "\n\n", 2)
		if len(parts) == 2 {
			errorBox := ui.Current.Help.Copy().
				BorderForeground(lipgloss.Color("#FF4040"))

			header := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FF4040")).
				Render(parts[0])

			details := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#999999")).
				Render(parts[1])

			return errorBox.Render(fmt.Sprintf("%s\n\n%s", header, details))
		}
	}
	return ui.Current.Error.Render(msg)
}

func ParseFlags(version string) (*Config, error) {
	flags := flag.NewFlagSet("keepalive", flag.ExitOnError)
	flags.Usage = func() {
		model := ui.InitialModel()
		model.ShowHelp = true
		fmt.Print(model.View())
	}

	duration := flags.String("duration", "", "Duration to keep system alive (e.g., \"2h30m\")")
	flags.StringVar(duration, "d", "", "Duration to keep system alive (e.g., \"2h30m\")")
	showVersion := flags.Bool("version", false, "Show version information")
	flags.BoolVar(showVersion, "v", false, "Show version information")

	if err := flags.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		return nil, err
	}

	if *showVersion {
		fmt.Printf("Keep-Alive Version: %s\n", version)
		os.Exit(0)
	}

	var minutes int
	if *duration != "" {
		d, err := util.ParseDuration(*duration)
		if err != nil {
			fmt.Println(formatError(err))
			os.Exit(1)
		}
		minutes = int(d.Minutes())
	}

	return &Config{
		Duration:    minutes,
		ShowVersion: *showVersion,
	}, nil
}
