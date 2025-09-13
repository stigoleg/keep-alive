//go:build linux

package platform

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// linuxKeepAlive implements the KeepAlive interface for Linux
type linuxKeepAlive struct {
	mu               sync.Mutex
	cmd              *exec.Cmd
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	isRunning        bool
	activityTick     *time.Ticker
	activeMethod     string
	prevDPMS         string
	prevTimeout      int
	prevCycle        int
	prevGSettings    map[string]string
	xdotoolAvailable bool
	xsetAvailable    bool
}

// trySystemdInhibit attempts to use systemd-inhibit
func trySystemdInhibit(ctx context.Context) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "systemd-inhibit", "--what=idle:sleep:handle-lid-switch",
		"--who=keep-alive", "--why=Prevent system sleep", "--mode=block",
		"sleep", "infinity")
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

// tryXsetMethod attempts to use xset to prevent screen sleep
func tryXsetMethod() error {
	// Disable screen saver
	if err := exec.Command("xset", "s", "off").Run(); err != nil {
		return err
	}
	// Disable DPMS (Display Power Management Signaling)
	if err := exec.Command("xset", "-dpms").Run(); err != nil {
		return err
	}
	return nil
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func readXsetState() (dpms string, timeout int, cycle int, err error) {
	out, err := exec.Command("xset", "-q").CombinedOutput()
	if err != nil {
		return "", 0, 0, err
	}
	lines := strings.Split(string(out), "\n")
	dpms = "on"
	timeout = -1
	cycle = -1
	for _, line := range lines {
		if strings.Contains(line, "DPMS is") {
			if strings.Contains(line, "Disabled") {
				dpms = "off"
			} else {
				dpms = "on"
			}
		}
		if strings.Contains(line, "timeout:") && strings.Contains(line, "cycle:") {
			var t, c int
			_, _ = fmt.Sscanf(line, "%*s %*s %d %*s %d", &t, &c)
			if t > 0 || c > 0 {
				timeout, cycle = t, c
			}
		}
	}
	return dpms, timeout, cycle, nil
}

func gsettingsGet(schema, key string) (string, error) {
	out, err := exec.Command("gsettings", "get", schema, key).CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Start initiates the keep-alive functionality
func (k *linuxKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	// Create a cancellable context
	ctx, k.cancel = context.WithCancel(ctx)

	// Capability probing and selection
	hasSystemd := hasCommand("systemd-inhibit")
	hasXset := hasCommand("xset") && os.Getenv("DISPLAY") != ""
	hasGsettings := hasCommand("gsettings")
	k.xdotoolAvailable = hasCommand("xdotool")
	k.xsetAvailable = hasXset

	if hasSystemd {
		cmd, err := trySystemdInhibit(ctx)
		if err == nil {
			k.cmd = cmd
			k.activeMethod = "systemd-inhibit"
			log.Printf("linux: active method: %s", k.activeMethod)
			k.wg.Add(1)
			go func() {
				defer k.wg.Done()
				k.cmd.Wait()
			}()
		} else {
			log.Printf("linux: systemd-inhibit failed: %v", err)
		}
	}

	if k.cmd == nil && hasXset {
		if dpms, t, c, err := readXsetState(); err == nil {
			k.prevDPMS, k.prevTimeout, k.prevCycle = dpms, t, c
		} else {
			log.Printf("linux: failed reading xset state: %v", err)
		}
		if err := tryXsetMethod(); err == nil {
			k.activeMethod = "xset"
			log.Printf("linux: active method: %s (DISPLAY=%s, xdotool=%v)", k.activeMethod, os.Getenv("DISPLAY"), k.xdotoolAvailable)
		} else {
			log.Printf("linux: xset method failed: %v", err)
		}
	}

	if k.activeMethod == "" && hasGsettings {
		k.prevGSettings = make(map[string]string)
		toSet := []struct{ schema, key, value string }{
			{"org.gnome.desktop.session", "idle-delay", "0"},
			{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-ac-type", "'nothing'"},
			{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-battery-type", "'nothing'"},
		}
		for _, s := range toSet {
			if prev, err := gsettingsGet(s.schema, s.key); err == nil {
				k.prevGSettings[s.schema+" "+s.key] = prev
			}
			if err := exec.Command("gsettings", "set", s.schema, s.key, s.value).Run(); err != nil {
				k.cancel()
				return fmt.Errorf("linux: failed to set gsettings %s %s: %w", s.schema, s.key, err)
			}
		}
		k.activeMethod = "gsettings"
		log.Printf("linux: active method: %s", k.activeMethod)
	}

	if k.cmd == nil && k.activeMethod == "" {
		k.cancel()
		return fmt.Errorf("linux: no keep-alive method available (systemd-inhibit, xset with X11, or GNOME gsettings)")
	}

	// Start periodic activity simulation
	k.activityTick = time.NewTicker(30 * time.Second)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.activityTick.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-k.activityTick.C:
				// Simulate user activity if available
				if k.xdotoolAvailable {
					exec.Command("xdotool", "mousemove_relative", "1", "0").Run()
					exec.Command("xdotool", "mousemove_relative", "-1", "0").Run()
				}
			}
		}
	}()

	k.isRunning = true
	return nil
}

// Stop terminates the keep-alive functionality
func (k *linuxKeepAlive) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.isRunning {
		return nil
	}

	if k.cancel != nil {
		k.cancel()
	}

	if k.cmd != nil && k.cmd.Process != nil {
		k.cmd.Process.Kill()
	}

	// Restore previous settings based on active method
	if k.activeMethod == "xset" && k.xsetAvailable {
		// Restore DPMS state
		if k.prevDPMS == "off" {
			exec.Command("xset", "-dpms").Run()
		} else if k.prevDPMS == "on" {
			exec.Command("xset", "+dpms").Run()
		}
		// Restore saver timeout when known
		if k.prevTimeout > 0 {
			exec.Command("xset", "s", fmt.Sprintf("%d", k.prevTimeout), fmt.Sprintf("%d", k.prevCycle)).Run()
		}
	} else if k.xsetAvailable {
		// Best-effort defaults when not using xset
		exec.Command("xset", "s", "on").Run()
		exec.Command("xset", "+dpms").Run()
	}

	if k.activeMethod == "gsettings" {
		for key, val := range k.prevGSettings {
			parts := strings.SplitN(key, " ", 2)
			if len(parts) == 2 {
				exec.Command("gsettings", "set", parts[0], parts[1], val).Run()
			}
		}
	}

	// Wait for all goroutines to finish
	k.wg.Wait()

	k.isRunning = false
	return nil
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &linuxKeepAlive{}, nil
}
