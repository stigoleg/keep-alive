package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/service"
)

func (a *App) serviceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Run keepalive in the background at every login",
		Long: `Run keepalive in the background, started at every login: a LaunchAgent on
macOS, a systemd user unit on Linux (an autostart entry without systemd) and a
Task Scheduler task on Windows. Only one keepalive runs at a time; control the
background one with "keepalive status", "keepalive stop", "keepalive active"
and "keepalive extend".`,
		Example: `  keepalive service install --schedule "Mon-Fri 08:00-16:00" -a
  keepalive service status
  keepalive service uninstall`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(a.serviceInstallCommand(), a.serviceUninstallCommand(), a.serviceStatusCommand())
	return cmd
}

func (a *App) manager() (service.Manager, error) {
	m, err := a.ServiceManager()
	if err != nil {
		return nil, runtimeErr(err, "keepalive can install a login service on macOS, Linux and Windows")
	}
	return m, nil
}

func (a *App) serviceInstallCommand() *cobra.Command {
	var sf sessionFlags
	cmd := &cobra.Command{
		Use:   "install [flags]",
		Short: "Install and start the login service",
		Long: `Install keepalive as a login service and start it now. It takes the same
session flags as keepalive itself; they are checked the same way and stored in
the service. Settings you do not pass here are read from the config file each
time the service starts, so later "keepalive config" edits apply from its next
start. Installing again replaces the previous service.`,
		Example: `  keepalive service install
  keepalive service install --schedule "Mon-Fri 08:00-16:00" -a
  keepalive service install --schedule "weekdays 08:00-16:00" --battery 20`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := a.plan(cmd, &sf, false); err != nil {
				return err
			}
			args, err := serviceArgs(cmd.Flags())
			if err != nil {
				return err
			}
			exe, warning, err := a.ResolveExecutable()
			if err != nil {
				return runtimeErr(err, service.Hint(err))
			}
			if warning != "" {
				fmt.Fprintf(a.Stderr, "keepalive: warning: %s\n", warning)
			}
			m, err := a.manager()
			if err != nil {
				return err
			}
			spec := service.Spec{Executable: exe, Args: args, LogPath: serviceLogPath()}
			if err := m.Install(spec); err != nil {
				return runtimeErr(err, service.Hint(err))
			}
			st, err := m.Status()
			if err != nil {
				st = service.State{Detail: err.Error()}
			}
			a.printInstalled(m, spec, st, cmd.Flags())
			return nil
		},
	}
	addSessionFlags(cmd.Flags(), &sf)
	return cmd
}

// serviceArgs serializes the flags given on the command line, and only
// those, so everything else follows the config file at run time.
func serviceArgs(fs *pflag.FlagSet) ([]string, error) {
	var args []string
	var err error
	fs.Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "plain", "help":
			return
		case "config":
			p, aerr := filepath.Abs(f.Value.String())
			if aerr != nil {
				err = aerr
				return
			}
			args = append(args, "--config="+p)
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			for _, v := range sv.GetSlice() {
				args = append(args, "--"+f.Name+"="+v)
			}
			return
		}
		if f.Value.Type() == "bool" {
			if v, _ := strconv.ParseBool(f.Value.String()); v {
				args = append(args, "--"+f.Name)
				return
			}
		}
		args = append(args, "--"+f.Name+"="+f.Value.String())
	})
	if err != nil {
		return nil, runtimeErr(err, "")
	}
	return append(args, "--origin", ipc.OriginService), nil
}

// serviceLogPath receives the service's own output where the manager
// supports it (launchd).
func serviceLogPath() string {
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Logs", "keepalive", "service.log")
		}
	}
	if p, err := logging.DefaultPath(); err == nil {
		return filepath.Join(filepath.Dir(p), "service.log")
	}
	return ""
}

func (a *App) printInstalled(m service.Manager, spec service.Spec, st service.State, fs *pflag.FlagSet) {
	w := a.Stdout
	fmt.Fprintf(w, "installed the login service (%s)\n", m.Name())
	if st.Path != "" {
		fmt.Fprintf(w, "  file:   %s\n", st.Path)
	}
	fmt.Fprintf(w, "  runs:   %s\n", commandLine(spec))
	if st.Detail != "" {
		fmt.Fprintf(w, "  state:  %s\n", st.Detail)
	}
	if spec.LogPath != "" && m.Name() == "launchd" {
		fmt.Fprintf(w, "  output: %s\n", spec.LogPath)
	}
	fmt.Fprintln(w, "\nSettings not given as flags here come from the config file each time the")
	fmt.Fprintln(w, "service starts, so later config edits apply from its next start.")
	if active, _ := fs.GetBool("active"); runtime.GOOS == "darwin" && active {
		fmt.Fprintln(w, `macOS will ask to allow "keepalive" under Privacy & Security → Accessibility the first time it simulates activity.`)
	}
	fmt.Fprintln(w, `Check it with "keepalive service status" or "keepalive status".`)
}

func commandLine(spec service.Spec) string {
	parts := []string{quoteArg(spec.Executable), "--plain"}
	for _, a := range spec.Args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'\\$&|;<>()*?") {
		return s
	}
	return strconv.Quote(s)
}

func (a *App) serviceUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the login service",
		Long: `Stop the keepalive the login service started, if it is running, and remove
the service so it no longer starts at login. A keepalive started in a
terminal is left alone.`,
		Example: `  keepalive service uninstall`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := a.manager()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			stopped := a.stopServiceInstance(ctx)
			err = m.Uninstall()
			if errors.Is(err, service.ErrNotInstalled) {
				fmt.Fprintln(a.Stdout, "the login service is not installed; nothing to remove")
				return nil
			}
			if err != nil {
				return runtimeErr(err, service.Hint(err))
			}
			msg := fmt.Sprintf("removed the login service (%s)", m.Name())
			if stopped > 0 {
				msg += fmt.Sprintf(" and stopped its keepalive (pid %d)", stopped)
			}
			fmt.Fprintln(a.Stdout, msg)
			return nil
		},
	}
}

// stopServiceInstance stops the running keepalive if the service started
// it, and returns its pid (0 when nothing was stopped).
func (a *App) stopServiceInstance(ctx context.Context) int {
	ctx, cancel := context.WithTimeout(ctx, stopWait)
	defer cancel()
	c, err := ipc.Dial(ctx)
	if err != nil {
		return 0
	}
	st, err := c.Status(ctx)
	if err != nil || st.Origin != ipc.OriginService {
		return 0
	}
	if err := c.Stop(ctx); err != nil {
		return 0
	}
	if err := waitGone(ctx, stopWait); err != nil {
		fmt.Fprintf(a.Stderr, "keepalive: warning: %v\n", err)
	}
	return st.PID
}

func (a *App) serviceStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "Show whether the login service is installed and running",
		Long:    `Show the login service as the service manager sees it, and what the keepalive it runs is doing.`,
		Example: `  keepalive service status`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := a.manager()
			if err != nil {
				return err
			}
			st, err := m.Status()
			if err != nil {
				return runtimeErr(err, service.Hint(err))
			}
			w := a.Stdout
			fmt.Fprintf(w, "login service (%s): %s\n", m.Name(), st.Detail)
			if st.Path != "" {
				fmt.Fprintf(w, "  file: %s\n", st.Path)
			}
			if !st.Installed {
				fmt.Fprintln(w, `Install it with "keepalive service install".`)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			c, err := ipc.Dial(ctx)
			if err != nil {
				fmt.Fprintln(w, "\nno keepalive is running")
				return nil
			}
			ist, err := c.Status(ctx)
			if err != nil {
				return controlErr(err)
			}
			fmt.Fprintln(w)
			fmt.Fprint(w, output.StatusText(statusInfo(ist), a.Now()))
			return nil
		},
	}
}
