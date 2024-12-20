package config

import (
	"flag"
	"fmt"
	"os"
	"github.com/stigoleg/keep-alive/internal/ui"
	"github.com/stigoleg/keep-alive/internal/util"
)

type Config struct {
	Duration    int
	ShowVersion bool
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
			return nil, fmt.Errorf("error parsing duration: %v", err)
		}
		minutes = int(d.Minutes())
	}

	return &Config{
		Duration:    minutes,
		ShowVersion: *showVersion,
	}, nil
}
