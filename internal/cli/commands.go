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
)

const rootLong = `keepalive keeps your computer awake, and optionally your chat status
(Slack, Teams) active, for a duration, until a time of day, or until the
battery runs low.

On a terminal it opens an interactive UI; otherwise (or with --plain/--json)
it runs headless and prints one line per event.

Settings come from flags, KEEPALIVE_* environment variables and the config
file (see 'keepalive config'), in that order of precedence.`

const rootExample = `  keepalive                     interactive UI
  keepalive -d 2h30m            keep awake for 2 hours 30 minutes
  keepalive -d 150              keep awake for 150 minutes
  keepalive -c 22:00            keep awake until 10 PM
  keepalive -b 20               keep awake until the battery is at 20%
  keepalive -d 20 -b 65 -a      stop after 20 minutes or at 65% battery; simulate activity
  keepalive --plain --json -d 1h  headless, NDJSON events`

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
	addSessionFlags(root.Flags(), &sf)
	root.Flags().BoolVar(&sf.replace, "replace", false, "stop a keepalive that is already running and take over")
	root.Flags().String("origin", "", "who started keepalive: terminal or service (set by the login service)")
	_ = root.Flags().MarkHidden("origin")
	root.Flags().BoolP("version", "v", false, "print the version and exit")
	root.PersistentFlags().String("config", "", "config file (default: see 'keepalive config path')")

	root.AddCommand(
		a.runCommand(),
		a.doctorCommand(),
		a.statusCommand(),
		a.stopCommand(),
		a.activeCommand(),
		a.extendCommand(),
		a.serviceCommand(),
		a.configCommand(),
		&cobra.Command{
			Use:   "version",
			Short: "Print the version",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, _ []string) {
				fmt.Fprintf(cmd.OutOrStdout(), "keepalive %s\n", a.Version)
			},
		},
	)
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
		Args:  cobra.ArbitraryArgs,
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
	return cmd
}

func (a *App) configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the config file",
		Long: `Manage the keepalive config file.

Precedence: flag > KEEPALIVE_* environment variable > config file > default.
Keys are the long flag names in snake_case (active_idle for --active-idle).`,
		Args: cobra.NoArgs,
	}

	path := &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args: cobra.NoArgs,
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
