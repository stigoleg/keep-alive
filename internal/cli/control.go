package cli

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/config"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// stopWait bounds how long `keepalive stop` waits for the instance to exit.
const stopWait = 10 * time.Second

// errNoInstance is exit code 3 for the control commands.
func errNoInstance() error {
	return &ExitError{
		Code: ExitNoInstance,
		Err:  errors.New("no keepalive is running"),
		Hint: `start one with "keepalive", or run it at login with "keepalive service install"`,
	}
}

// dial connects to the running instance.
func dial(ctx context.Context) (*ipc.Client, error) {
	c, err := ipc.Dial(ctx)
	if errors.Is(err, ipc.ErrNotRunning) {
		return nil, errNoInstance()
	}
	if err != nil {
		return nil, runtimeErr(err, "")
	}
	return c, nil
}

func controlErr(err error) error {
	if errors.Is(err, ipc.ErrNotRunning) {
		return errNoInstance()
	}
	return runtimeErr(err, "")
}

func statusInfo(st ipc.Status) output.StatusInfo {
	return output.StatusInfo{PID: st.PID, Version: st.Version, Origin: st.Origin, Snapshot: st.Snapshot}
}

func (a *App) statusCommand() *cobra.Command {
	var asJSON, follow bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show what the running keepalive is doing",
		Long: `Show the running keepalive: how long it keeps the machine awake, the work
hours, activity simulation, the power hold, the battery and who started it.

Exits with status 3 when no keepalive is running, so scripts can test for
one. --follow keeps printing its events until it stops.`,
		Example: `  keepalive status
  keepalive status --json
  keepalive status --follow`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), stopSignals()...)
			defer stop()
			c, err := dial(ctx)
			if err != nil {
				return err
			}
			var events <-chan ipc.Event
			if follow { // subscribe first so nothing between status and stream is lost
				if events, err = c.Subscribe(ctx); err != nil {
					return controlErr(err)
				}
			}
			st, err := c.Status(ctx)
			if err != nil {
				return controlErr(err)
			}
			lines := output.NewJSONLines(a.Stdout)
			if asJSON {
				if err := lines.Write(statusInfo(st)); err != nil {
					return runtimeErr(err, "")
				}
			} else {
				fmt.Fprint(a.Stdout, output.StatusText(statusInfo(st), a.Now()))
			}
			if !follow {
				return nil
			}
			human := output.NewFollow(a.Stdout, a.StdoutTTY && a.colorAllowed())
			for ev := range events {
				if asJSON {
					err = lines.Write(ev.JSONEvent)
				} else {
					err = human.Print(ev.JSONEvent)
				}
				if err != nil {
					return runtimeErr(err, "")
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON: one status object (and NDJSON events with --follow)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing events until the instance stops")
	return cmd
}

func (a *App) stopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running keepalive",
		Long: `Stop the running keepalive, wherever it was started: a terminal, the login
service or "keepalive run" (the command itself keeps running). The machine
may sleep again once it has stopped.

A login service started this way stays stopped until the next login; use
"keepalive service uninstall" to remove it.`,
		Example: `  keepalive stop`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := dial(ctx)
			if err != nil {
				return err
			}
			pid := 0
			if st, err := c.Status(ctx); err == nil {
				pid = st.PID
			}
			if err := c.Stop(ctx); err != nil {
				return controlErr(err)
			}
			if err := waitGone(ctx, stopWait); err != nil {
				return err
			}
			if pid > 0 {
				fmt.Fprintf(a.Stdout, "stopped keepalive (pid %d)\n", pid)
			} else {
				fmt.Fprintln(a.Stdout, "stopped keepalive")
			}
			return nil
		},
	}
}

// waitGone waits until nothing answers on the control socket.
func waitGone(ctx context.Context, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		if _, err := ipc.Dial(ctx); errors.Is(err, ipc.ErrNotRunning) {
			return nil
		}
		if time.Now().After(deadline) {
			return runtimeErr(fmt.Errorf("keepalive was asked to stop but is still running after %s", limit),
				`check "keepalive status"; it may be finishing a slow notification or power release`)
		}
		select {
		case <-ctx.Done():
			return runtimeErr(ctx.Err(), "")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (a *App) activeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "active on|off",
		Short: "Turn activity simulation on or off in the running keepalive",
		Long: `Turn activity simulation on or off in the running keepalive without
restarting it. With it on, keepalive simulates input after you have been idle
for a while, so chat apps (Teams, Slack) keep showing you as active.`,
		Example: `  keepalive active on
  keepalive active off`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var on bool
			switch strings.ToLower(args[0]) {
			case "on", "true", "yes", "1":
				on = true
			case "off", "false", "no", "0":
			default:
				return usageErr(fmt.Errorf("unknown value %q", args[0]), `use "keepalive active on" or "keepalive active off"`)
			}
			ctx := cmd.Context()
			c, err := dial(ctx)
			if err != nil {
				return err
			}
			if err := c.SetActive(ctx, on); err != nil {
				return controlErr(err)
			}
			st, err := c.Status(ctx)
			if err != nil {
				return controlErr(err)
			}
			snap := st.Snapshot
			switch {
			case !snap.Running:
				fmt.Fprintln(a.Stdout, "no session is running in the interactive UI; nothing changed")
			case !on:
				fmt.Fprintln(a.Stdout, "activity simulation off")
			case !snap.InWindow && snap.NextChange != nil:
				next, _ := time.Parse(time.RFC3339Nano, *snap.NextChange)
				fmt.Fprintf(a.Stdout, "activity simulation on (starts with the work hours at %s)\n", session.ClockText(a.Now(), next.Local()))
			default:
				fmt.Fprintln(a.Stdout, "activity simulation on")
			}
			return nil
		},
	}
}

func (a *App) extendCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extend DURATION",
		Short: "Move the end of the running keepalive",
		Long: `Move the end of the running keepalive by DURATION: minutes ("30") or a
duration ("1h", "15m"). A negative value ("-15m") shortens it, but never to
less than one minute from now. Only sessions with an end (-d or -c) can be
extended.`,
		Example: `  keepalive extend 30m
  keepalive extend 1h
  keepalive extend -15m`,
		// Flag parsing would read "-15m" as flags.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			if len(args) != 1 {
				return usageErr(errors.New("extend needs one duration"), `e.g. "keepalive extend 30m" or "keepalive extend -15m"`)
			}
			d, err := parseExtend(args[0])
			if err != nil {
				return usageErr(err, `use minutes ("30") or a duration ("1h", "-15m")`)
			}
			ctx := cmd.Context()
			c, err := dial(ctx)
			if err != nil {
				return err
			}
			st, err := c.Status(ctx)
			if err != nil {
				return controlErr(err)
			}
			if !st.Snapshot.Running || st.Snapshot.EndsAt == nil {
				return runtimeErr(errors.New("the running keepalive has no end time to move"),
					`start it with one, e.g. "keepalive --replace -d 2h" or "keepalive --replace -c 17:00"`)
			}
			if err := c.Extend(ctx, d); err != nil {
				return controlErr(err)
			}
			if st, err = c.Status(ctx); err != nil {
				return controlErr(err)
			}
			snap := st.Snapshot
			if snap.EndsAt != nil && snap.Remaining != nil {
				end, _ := time.Parse(time.RFC3339Nano, *snap.EndsAt)
				fmt.Fprintf(a.Stdout, "keepalive now runs until %s (%s left)\n",
					session.ClockText(a.Now(), end.Local()), output.FormatDuration(time.Duration(*snap.Remaining)*time.Second))
			}
			return nil
		},
	}
	return cmd
}

// parseExtend reads "30", "1h", "+15m" or "-15m".
func parseExtend(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	sign := time.Duration(1)
	switch {
	case strings.HasPrefix(s, "-"):
		sign, s = -1, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	d, err := config.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return sign * d, nil
}
