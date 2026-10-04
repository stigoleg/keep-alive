// Package cli is the keepalive command line: the cobra command tree, the
// configuration precedence, mode selection (TUI or headless) and exit codes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/clock"
	"github.com/stigoleg/keep-alive/v2/internal/config"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/session"
	"github.com/stigoleg/keep-alive/v2/internal/util"
)

// App is one invocation of the CLI. Its fields are the process environment,
// so tests can substitute every one of them.
type App struct {
	Version                        string
	Stdin                          io.Reader
	Stdout, Stderr                 io.Writer
	StdinTTY, StdoutTTY, StderrTTY bool
	LookupEnv                      func(string) (string, bool)
	Now                            func() time.Time
	Battery                        func() (platform.BatteryStatus, error)
	DefaultConfigPath              func() (string, error)

	// runSession executes a resolved plan; tests replace it.
	runSession func(ctx context.Context, p *Plan) error
}

// Plan is a fully resolved request to run a session.
type Plan struct {
	Session   session.Config
	TUI       bool // interactive UI instead of headless output
	JSON      bool // NDJSON events (headless only)
	AutoStart bool // TUI: start the session right away, as v1 did for -d/-c/-b
	Notify    bool // phase 4
	Logging   logging.Options
	Config    config.Resolved
}

// Main runs the CLI against the real process environment and returns the exit
// code.
func Main(version string) int {
	return NewApp(version).Execute(os.Args[1:])
}

// NewApp returns an App wired to the real process.
func NewApp(version string) *App {
	return &App{
		Version:           ResolveVersion(version),
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		StdinTTY:          isTerminal(os.Stdin),
		StdoutTTY:         isTerminal(os.Stdout),
		StderrTTY:         isTerminal(os.Stderr),
		LookupEnv:         os.LookupEnv,
		Now:               time.Now,
		Battery:           platform.GetBatteryStatus,
		DefaultConfigPath: config.DefaultPath,
	}
}

// NewRootCommand returns the command tree; cmd/gen-docs uses it for man
// pages and completions.
func NewRootCommand(version string) *cobra.Command {
	return NewApp(version).Command()
}

// Execute runs the command line and returns the exit code. Errors are printed
// as one "keepalive: error: …" line plus an optional hint.
func (a *App) Execute(args []string) int {
	cmd := a.Command()
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		printError(a.Stderr, err, a.color())
	}
	return ExitCode(err)
}

func (a *App) color() bool {
	if v, ok := a.LookupEnv("NO_COLOR"); ok && v != "" {
		return false
	}
	return a.StderrTTY
}

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// realDeps are the production session dependencies.
func realDeps() session.Deps {
	return session.Deps{
		Clock:    clock.Real(),
		Power:    power.New(),
		Activity: activity.New(),
		Battery:  platform.GetBatteryStatus,
	}
}

// sessionFlags holds the flag values that are not config keys; config keys
// are read back from the flag set by config.Resolve.
type sessionFlags struct {
	duration config.DurationValue
	clock    string
	pids     []int
	while    string
	json     bool
	plain    bool
}

// addSessionFlags defines the session flags shared by the root command,
// `run`, `service install` and `config show`.
func addSessionFlags(fs *pflag.FlagSet, f *sessionFlags) {
	def := config.Defaults()
	fs.VarP(&f.duration, "duration", "d", `keep awake for this long: minutes ("150") or a duration ("2h30m", "45m"); at least 1m`)
	fs.StringVarP(&f.clock, "clock", "c", "", `keep awake until this time of day ("22:00", "10:00PM"); alias --until`)
	fs.IntP("battery", "b", 0, "stop when the battery is at or below this percentage (1-100)")
	fs.BoolP("active", "a", false, "simulate user activity so chat apps (Slack, Teams) keep you active")
	fs.Duration("active-idle", def.ActiveIdle, "idle time before activity is simulated (at least 10s)")
	fs.Duration("active-interval", def.ActiveInterval, "mean gap between simulated activity bursts (at least 5s)")
	fs.Bool("active-keys", false, "also send a harmless key press with each activity burst")
	fs.String("schedule", "", "only keep awake during this schedule (not applied yet)")
	fs.IntSliceVar(&f.pids, "pid", nil, "keep awake while these processes run (not applied yet)")
	fs.StringVar(&f.while, "while", "", "keep awake while a process with this name runs (not applied yet)")
	fs.Bool("keep-display", def.KeepDisplay, "keep the display on too (false keeps only the system awake)")
	fs.BoolVar(&f.json, "json", false, "print NDJSON events on stdout (implies --plain)")
	fs.BoolVar(&f.plain, "plain", false, "run headless even on a terminal")
	fs.Bool("notify", false, "show a desktop notification when the session ends (not applied yet)")
	fs.BoolP("log", "l", false, "write a debug log (the path is printed at start)")
	fs.String("log-file", "", "log file path (default: keepalive/keepalive.log in the user cache directory)")
	fs.SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == "until" {
			name = "clock"
		}
		return pflag.NormalizedName(name)
	})
}

// configPath returns the --config path or the default one.
func (a *App) configPath(cmd *cobra.Command) (path string, explicit bool, err error) {
	if f := cmd.Flags().Lookup("config"); f != nil && f.Changed {
		return f.Value.String(), true, nil
	}
	path, err = a.DefaultConfigPath()
	return path, false, err
}

func (a *App) resolveConfig(cmd *cobra.Command) (config.Resolved, error) {
	path, explicit, err := a.configPath(cmd)
	if err != nil {
		return config.Resolved{}, runtimeErr(err, "pass --config to choose a file")
	}
	res, err := config.Resolve(config.Options{Path: path, PathExplicit: explicit, Env: a.LookupEnv, Flags: cmd.Flags()})
	if err != nil {
		return res, usageErr(err, "check the value, or run 'keepalive config show'")
	}
	return res, nil
}

// plan turns flags, env and config into a session request and validates it.
func (a *App) plan(cmd *cobra.Command, f *sessionFlags) (*Plan, error) {
	res, err := a.resolveConfig(cmd)
	if err != nil {
		return nil, err
	}
	fs := cmd.Flags()
	s := session.Config{
		Duration:         res.Duration,
		BatteryThreshold: res.Battery,
		Active:           res.Active,
		Activity:         activity.Config{IdleThreshold: res.ActiveIdle, Interval: res.ActiveInterval, Keys: res.ActiveKeys},
		KeepDisplay:      res.KeepDisplay,
		ScheduleSpec:     res.Schedule,
		WatchPIDs:        f.pids,
		WatchProcess:     f.while,
	}

	if fs.Changed("clock") {
		if fs.Changed("duration") {
			return nil, usageErr(errors.New("--duration and --clock cannot be used together"), "use one of them, e.g. -d 2h or -c 22:00")
		}
		until, err := util.NextClockTime(f.clock, a.Now())
		if err != nil {
			return nil, usageErr(err, "")
		}
		s.Until, s.Duration = until, 0 // an explicit --clock wins over a configured duration
	}

	if s.BatteryThreshold > 0 {
		if err := a.checkBattery(s.BatteryThreshold); err != nil {
			return nil, err
		}
	}

	return &Plan{
		Session:   s,
		TUI:       a.StdinTTY && a.StdoutTTY && !f.plain && !f.json,
		JSON:      f.json,
		AutoStart: s.Duration > 0 || !s.Until.IsZero() || s.BatteryThreshold > 0,
		Notify:    res.Notify,
		Logging:   logging.Options{Enabled: res.Log, Debug: res.Log, Path: res.LogFile},
		Config:    res,
	}, nil
}

func (a *App) checkBattery(threshold int) error {
	st, err := a.Battery()
	if err != nil || !st.Available {
		if err == nil {
			err = errors.New("not available")
		}
		return usageErr(fmt.Errorf("battery threshold %d%% set, but no battery was found (%v)", threshold, err),
			"--battery only works on machines with a battery; remove it (or the battery setting)")
	}
	if st.Percentage <= threshold {
		return usageErr(fmt.Errorf("battery threshold must be below the current level (current %d%%, threshold %d%%)", st.Percentage, threshold),
			"choose a lower --battery value")
	}
	return nil
}
