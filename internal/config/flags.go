package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stigoleg/keep-alive/internal/ui"
	"github.com/stigoleg/keep-alive/internal/util"
)

type Config struct {
	Duration         int
	Clock            time.Time
	ShowVersion      bool
	SimulateActivity bool
	EnableLogging    bool
}

func formatError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "Invalid duration format:") || strings.Contains(msg, "invalid time format:") {
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

// ParseFlags parses command line flags and returns the configuration
func ParseFlags(version string) (*Config, error) {
	return ParseFlagsWithNow(version, time.Now())
}

// ParseFlagsWithNow is like ParseFlags but accepts a custom "now" time
// This is primarily used for testing to ensure consistent results
func ParseFlagsWithNow(version string, now time.Time) (*Config, error) {
	flags := flag.NewFlagSet("keepalive", flag.ExitOnError)
	flags.Usage = func() {
		model := ui.InitialModel()
		model.ShowHelp = true
		model.SetVersion(version)
		fmt.Print(model.View())
	}

	duration := flags.String("duration", "", "Duration to keep system alive (e.g., \"2h30m\")")
	flags.StringVar(duration, "d", "", "Duration to keep system alive (e.g., \"2h30m\")")

	clock := flags.String("clock", "", "Time to keep system alive until (e.g., \"22:00\" or \"10:00PM\")")
	flags.StringVar(clock, "c", "", "Time to keep system alive until (e.g., \"22:00\" or \"10:00PM\")")

	showVersion := flags.Bool("version", false, "Show version information")
	flags.BoolVar(showVersion, "v", false, "Show version information")

	simulateActivity := flags.Bool("active", false, "Simulate activity to keep chat apps active")
	flags.BoolVar(simulateActivity, "a", false, "Simulate activity to keep chat apps active")

	enableLogging := flags.Bool("log", false, "Enable logging to debug.log file")
	flags.BoolVar(enableLogging, "l", false, "Enable logging to debug.log file")

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
	var clockTime time.Time

	if *duration != "" && *clock != "" {
		return nil, fmt.Errorf("cannot specify both duration (-d) and clock time (-c)")
	}

	if *duration != "" {
		d, err := util.ParseDuration(*duration)
		if err != nil {
			fmt.Println(formatError(err))
			os.Exit(1)
		}
		minutes = int(d.Minutes())
	} else if *clock != "" {
		t, err := util.ParseTimeStringWithNow(*clock, now)
		if err != nil {
			fmt.Println(formatError(err))
			os.Exit(1)
		}

		if t.Before(now) {
			// If the specified time is before now, assume it's for tomorrow
			t = t.Add(24 * time.Hour)
		}

		minutes = int(t.Sub(now).Minutes())
		clockTime = t
	}

	return &Config{
		Duration:         minutes,
		Clock:            clockTime,
		ShowVersion:      *showVersion,
		SimulateActivity: *simulateActivity,
		EnableLogging:    *enableLogging,
	}, nil
}
