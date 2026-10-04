package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/config"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/power"
)

const rootLong = `keepalive keeps your computer awake: for a while, until a time, during work
hours, while an app or a command runs, or until the battery runs low. With -a
it also simulates activity, so Teams and Slack keep showing you as active.

  keepalive                                     open the interactive UI
  keepalive -d 2h                               keep awake for 2 hours
  keepalive -c 17:00 -a                         keep awake until 17:00 and simulate activity
  keepalive --schedule "Mon-Fri 08:00-16:00" -a keep awake during work hours
  keepalive run -- make release                 keep awake while a command runs

On a terminal keepalive opens an interactive UI; otherwise, or with --plain or
--json, it runs headless and prints one line per event. Only one keepalive runs
at a time: control it with "keepalive status", "stop", "active" and "extend",
or start it at every login with "keepalive service install". "keepalive doctor"
checks what works on this machine.

Settings come from flags, KEEPALIVE_* environment variables and the config
file ("keepalive config"), in that order.`

const rootExample = `  keepalive -d 150                  keep awake for 150 minutes
  keepalive -b 20 -a                keep awake and simulate activity until the battery is at 20%
  keepalive --while zoom            keep awake while Zoom runs
  keepalive --pid 4242              keep awake until process 4242 exits
  keepalive --plain --json -d 1h    headless, with NDJSON events
  keepalive --replace -d 2h         take over from a keepalive that is already running`

// Command groups in the root help.
const (
	groupControl = "control"
	groupSetup   = "setup"
)

// Command builds the command tree.
func (a *App) Command() *cobra.Command {
	if a.runSession == nil {
		a.runSession = a.execute
	}
	if a.runChild == nil {
		a.runChild = a.executeRun
	}
	if a.logSetup == nil {
		a.logSetup = logging.Setup
	}
	if a.newPower == nil {
		a.newPower = power.New
	}
	if a.doctorFacts == nil {
		a.doctorFacts = a.collectDoctor
	}
	if a.probe == nil {
		a.probe = func(ctx context.Context) activity.ProbeResult { return activity.Probe(ctx, false) }
		a.probeWait = probeDelay
	}
	var sf sessionFlags
	root := &cobra.Command{
		Use:           "keepalive",
		Short:         "Keep your computer awake and your chat status active",
		Long:          rootLong,
		Example:       rootExample,
		Args:          rootArgs,
		Version:       a.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := a.plan(cmd, &sf, false)
			if err != nil {
				return err
			}
			if err := a.runSession(cmd.Context(), p); err != nil {
				var ee *ExitError
				if !errors.As(err, &ee) {
					err = runtimeErr(err, "")
				}
				return err
			}
			return nil
		},
	}
	root.SetIn(a.Stdin)
	root.SetOut(a.Stdout)
	root.SetErr(a.Stderr)
	root.SetVersionTemplate("keepalive {{.Version}}\n")
	root.SetFlagErrorFunc(flagError)
	root.SetUsageTemplate(usageTemplate)
	addSessionFlags(root.Flags(), &sf)
	root.Flags().BoolVar(&sf.replace, "replace", false, "stop a keepalive that is already running and take over")
	setGroup(root.Flags(), groupSession, "replace")
	root.Flags().String("origin", "", "who started keepalive: terminal or service (set by the login service)")
	_ = root.Flags().MarkHidden("origin")
	root.Flags().BoolP("version", "v", false, "print the version and exit")
	root.PersistentFlags().String("config", "", `config file (default: see "keepalive config path")`)
	root.AddGroup(
		&cobra.Group{ID: groupControl, Title: "Control the running keepalive:"},
		&cobra.Group{ID: groupSetup, Title: "Set up and check:"},
	)

	control := []*cobra.Command{a.statusCommand(), a.stopCommand(), a.activeCommand(), a.extendCommand()}
	for _, c := range control {
		c.GroupID = groupControl
	}
	setup := []*cobra.Command{a.doctorCommand(), a.serviceCommand(), a.configCommand()}
	for _, c := range setup {
		c.GroupID = groupSetup
	}
	root.AddCommand(a.runCommand(), &cobra.Command{
		Use:     "version",
		Short:   "Print the version",
		Long:    "Print the keepalive version (the same as keepalive --version).",
		Example: "  keepalive version",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "keepalive %s\n", a.Version)
		},
	})
	root.AddCommand(control...)
	root.AddCommand(setup...)
	root.InitDefaultCompletionCmd()
	if c, _, err := root.Find([]string{"completion"}); err == nil && c != root {
		c.Example = `  keepalive completion zsh > "${fpath[1]}/_keepalive"
  keepalive completion bash > /etc/bash_completion.d/keepalive
  keepalive completion fish > ~/.config/fish/completions/keepalive.fish`
	}
	return root
}

// rootArgs turns a command given to the root into a pointer to `run`.
func rootArgs(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return usageErr(fmt.Errorf("unexpected argument %q", args[0]),
		fmt.Sprintf(`did you mean "keepalive run -- %s"?`, strings.Join(args, " ")))
}

// flagError turns a pflag error into a concise usage error.
func flagError(_ *cobra.Command, err error) error {
	var ive *pflag.InvalidValueError
	if errors.As(err, &ive) {
		cause := errors.Unwrap(ive)
		var num *strconv.NumError
		if errors.As(cause, &num) {
			cause = fmt.Errorf("%q is not a whole number", ive.GetValue())
		}
		err = fmt.Errorf("--%s: %w", ive.GetFlag().Name, cause)
	}
	return usageErr(err, "run 'keepalive --help' for usage")
}

const runUsage = "usage: keepalive run [flags] -- command [args…]"

func (a *App) runCommand() *cobra.Command {
	var sf sessionFlags
	cmd := &cobra.Command{
		Use:   "run [flags] -- COMMAND [ARGS...]",
		Short: "Keep the machine awake while a command runs",
		Long: `Run a command and keep the machine awake until it exits. The command gets
the terminal (stdin, stdout and stderr); keepalive only writes a start line,
warnings and an unusual stop to stderr (NDJSON with --json). Ctrl+C and other
signals go to the command, and keepalive exits with the command's exit code.

Session flags work as for keepalive itself and go before "--". keepalive run
also works next to a keepalive that is already running.`,
		Example: `  keepalive run -- make release
  keepalive run -a -- ./long-test.sh --verbose
  keepalive run -b 20 -- rsync -a ~/photos nas:/backup`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			if dash != 0 || len(args) == 0 {
				return usageErr(errors.New(runUsage), `put the command after "--", e.g. keepalive run -- make release`)
			}
			p, err := a.plan(cmd, &sf, true)
			if err != nil {
				return err
			}
			p.Session.Command = filepath.Base(args[0])
			return a.runChild(cmd.Context(), p, args)
		},
	}
	addSessionFlags(cmd.Flags(), &sf)
	// run is always headless; --plain is accepted but means nothing here.
	_ = cmd.Flags().MarkHidden("plain")
	cmd.Flags().Lookup("json").Usage = "print NDJSON events instead of human lines"
	return cmd
}

func (a *App) configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the config file",
		Long: `Manage the keepalive config file, where you keep the settings you always want
(for example active = true or a work-hours schedule).

Precedence: flag > KEEPALIVE_* environment variable > config file > default.
Keys are the long flag names in snake_case (active_idle for --active-idle).`,
		Example: `  keepalive config init
  keepalive config show
  keepalive config path`,
		Args: cobra.NoArgs,
	}

	path := &cobra.Command{
		Use:     "path",
		Short:   "Print the config file path",
		Long:    "Print where keepalive reads its config file, whether or not it exists.",
		Example: "  keepalive config path\n  $EDITOR \"$(keepalive config path)\"",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, _, err := a.configPath(c)
			if err != nil {
				return runtimeErr(err, "")
			}
			fmt.Fprintln(c.OutOrStdout(), p)
			return nil
		},
	}

	var force bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Write a commented config file template",
		Long: `Write a config file with every setting commented out and explained, ready to
edit. It refuses to overwrite an existing file unless --force is given.`,
		Example: "  keepalive config init\n  keepalive config init --force",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, _, err := a.configPath(c)
			if err != nil {
				return runtimeErr(err, "")
			}
			if err := config.Init(p, force); err != nil {
				if errors.Is(err, config.ErrExists) {
					return runtimeErr(err, "use --force to overwrite it")
				}
				return runtimeErr(err, "")
			}
			fmt.Fprintf(c.OutOrStdout(), "wrote %s\n", p)
			return nil
		},
	}
	initCmd.Flags().BoolVar(&force, "force", false, "overwrite an existing file")

	var sf sessionFlags
	show := &cobra.Command{
		Use:   "show [flags]",
		Short: "Print the effective configuration and where each value comes from",
		Long: `Print the effective configuration as TOML. Each value is annotated with
its source: default, file, env or flag. Session flags may be given to see how
they combine with the environment and the file.`,
		Example: "  keepalive config show\n  keepalive config show -a --schedule \"Mon-Fri 08:00-16:00\"",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			res, err := a.resolveConfig(c)
			if err != nil {
				return err
			}
			fmt.Fprint(c.OutOrStdout(), res.TOML())
			return nil
		},
	}
	addSessionFlags(show.Flags(), &sf)

	cmd.AddCommand(path, initCmd, show)
	return cmd
}
