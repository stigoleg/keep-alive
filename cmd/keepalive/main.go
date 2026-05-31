package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/stigoleg/keep-alive/internal/config"
	"github.com/stigoleg/keep-alive/internal/keepalive"
	"github.com/stigoleg/keep-alive/internal/platform"
	"github.com/stigoleg/keep-alive/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	appVersion      = "1.5.3"
	shutdownTimeout = 5 * time.Second
)

var (
	cleanupOnce sync.Once
	keeperRef   *keepalive.Keeper
	logFile     *os.File
)

func main() {
	cfg, err := config.ParseFlags(appVersion)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.ShowVersion {
		fmt.Printf("Keep-Alive Version: %s\n", appVersion)
		return
	}

	if cfg.EnableLogging {
		f, err := tea.LogToFile("debug.log", "debug")
		if err != nil {
			fallbackPath := filepath.Join(os.TempDir(), "keepalive-debug.log")
			fallbackFile, fallbackErr := os.OpenFile(fallbackPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if fallbackErr != nil {
				log.Fatalf("failed to enable logging: primary error=%v fallback error=%v", err, fallbackErr)
			}
			logFile = fallbackFile
			log.SetOutput(fallbackFile)
			log.Printf("logging enabled via fallback file %s (primary debug.log unavailable: %v)", fallbackPath, err)
		} else {
			logFile = f
			log.SetOutput(f)
			if absPath, err := filepath.Abs("debug.log"); err == nil {
				log.Printf("logging enabled; writing debug logs to %s", absPath)
			} else {
				log.Printf("logging enabled; writing debug logs to debug.log")
			}
		}
	} else {
		log.SetOutput(io.Discard)
		logFile = nil
	}
	defer func() {
		if logFile != nil {
			logFile.Sync()
			logFile.Close()
		}
	}()

	var model ui.Model
	var batteryStatus platform.BatteryStatus
	if cfg.BatteryThreshold > 0 {
		status, err := platform.GetBatteryStatus()
		if err != nil {
			fmt.Fprint(os.Stderr, ui.ErrorBanner(fmt.Sprintf("battery status unavailable: %v", err)))
			os.Exit(1)
		}
		if status.Percentage <= cfg.BatteryThreshold {
			fmt.Fprint(os.Stderr, ui.ErrorBanner(fmt.Sprintf("battery threshold must be below current battery percentage (current: %d%%, threshold: %d%%)", status.Percentage, cfg.BatteryThreshold)))
			os.Exit(1)
		}
		batteryStatus = status
	}

	sigChan := make(chan os.Signal, 1)
	signals := getSignals()
	signal.Notify(sigChan, signals...)

	if cfg.Headless {
		headlessKeeper := keepalive.NewKeeper()
		keeperRef = headlessKeeper
		headlessKeeper.SetSimulateActivity(cfg.SimulateActivity)

		if cfg.SimulateActivity {
			activeStatus := platform.GetActivitySimulationStatus()
			if !activeStatus.Available {
				fmt.Printf("keep-alive: warning: activity simulation unavailable: %s\n", activeStatus.Message)
			}
		}

		depMessage := platform.GetDependencyMessage()
		if depMessage != "" {
			log.Printf("keep-alive: missing dependencies: %s", depMessage)
		}

		runHeadless(keeperRef, cfg, sigChan)
		executeCleanup(nil)
		return
	}

	if cfg.Duration > 0 || cfg.BatteryThreshold > 0 {
		model = ui.InitialModelWithLimits(cfg.Duration, cfg.BatteryThreshold, batteryStatus, cfg.SimulateActivity)
	} else {
		model = ui.InitialModel()
		model.SimulateActivity = cfg.SimulateActivity
	}
	model.SetVersion(appVersion)

	// Check for missing dependencies and store in model for TUI display
	depMessage := platform.GetDependencyMessage()
	if depMessage != "" {
		model.SetDependencyWarning(depMessage)
		log.Printf("linux: missing dependencies detected:\n%s", depMessage)
	}
	if cfg.SimulateActivity {
		activeStatus := platform.GetActivitySimulationStatus()
		if !activeStatus.Available {
			model.SetActivityWarning(activeStatus.Message)
			log.Printf("activity simulation unavailable: %s", activeStatus.Message)
		}
	}

	keeperRef = model.KeepAlive

	// Create program with signal handling
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithoutSignalHandler(),
	)

	// Handle first termination signal in a separate goroutine.
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v", sig)

		// Handle SIGTSTP (Ctrl+Z) - prevent suspension and initiate shutdown
		if isSIGTSTP(sig) {
			log.Printf("SIGTSTP received: preventing suspension and initiating graceful shutdown")
		}

		executeCleanup(p)
	}()

	if _, err := p.Run(); err != nil {
		log.Printf("Error running program: %v", err)
		os.Exit(1)
	}

	// Ensure cleanup runs on normal exit
	executeCleanup(nil)
}

// runHeadless runs the keep-alive logic directly without the TUI.
// It returns when the session completes (duration expires, battery
// threshold met, or a termination signal is received).
func runHeadless(keeper *keepalive.Keeper, cfg *config.Config, sigChan chan os.Signal) {
	fmt.Println("keep-alive: starting (headless mode)")

	keeper.SetSimulateActivity(cfg.SimulateActivity)
	if cfg.SimulateActivity {
		activeStatus := platform.GetActivitySimulationStatus()
		if !activeStatus.Available {
			fmt.Printf("keep-alive: warning: activity simulation unavailable: %s\n", activeStatus.Message)
		} else {
			fmt.Println("keep-alive: activity simulation enabled")
		}
	}

	var err error
	if cfg.Duration > 0 {
		d := time.Duration(cfg.Duration) * time.Minute
		fmt.Printf("keep-alive: starting timed session (%s)\n", d)
		err = keeper.StartTimed(d)
	} else {
		fmt.Println("keep-alive: starting indefinite session")
		err = keeper.StartIndefinite()
	}
	if err != nil {
		fmt.Fprint(os.Stderr, ui.ErrorBanner(fmt.Sprintf("failed to start: %v", err)))
		os.Exit(1)
	}

	// Start battery polling if threshold is set
	var batteryStop chan struct{}
	if cfg.BatteryThreshold > 0 {
		batteryStop = make(chan struct{})
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-batteryStop:
					return
				case <-ticker.C:
					status, err := platform.GetBatteryStatus()
					if err != nil {
						fmt.Printf("keep-alive: battery check failed: %v\n", err)
						continue
					}
					fmt.Printf("keep-alive: battery %d%% (threshold %d%%)\n",
						status.Percentage, cfg.BatteryThreshold)
					if status.Percentage <= cfg.BatteryThreshold {
						fmt.Printf("keep-alive: battery threshold reached (%d%% <= %d%%), stopping\n",
							status.Percentage, cfg.BatteryThreshold)
						keeper.Stop()
						return
					}
				}
			}
		}()
	}

	// Compute an overall deadline for the session duration timer
	sessionDeadline := make(chan struct{})
	if cfg.Duration > 0 {
		deadline := time.Duration(cfg.Duration) * time.Minute
		go func() {
			<-time.After(deadline)
			fmt.Println("keep-alive: duration elapsed, stopping")
			close(sessionDeadline)
		}()
	}

	// Wait for termination signal, duration expiry, or battery stop
	select {
	case sig := <-sigChan:
		fmt.Printf("keep-alive: received signal %v, stopping\n", sig)
		if isSIGTSTP(sig) {
			fmt.Println("keep-alive: preventing suspension and initiating graceful shutdown")
		}
	case <-sessionDeadline:
	}

	if batteryStop != nil {
		close(batteryStop)
	}

	keeper.Stop()
	fmt.Println("keep-alive: stopped (headless mode)")
}

// executeCleanup performs cleanup operations with timeout protection
func executeCleanup(p *tea.Program) {
	cleanupOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			defer close(done)

			if keeperRef != nil {
				if err := keeperRef.Stop(); err != nil {
					log.Printf("Error stopping keep-alive: %v", err)
				}
			}

			if logFile != nil {
				logFile.Sync()
			}
		}()

		select {
		case <-done:
			log.Printf("Cleanup completed successfully")
		case <-ctx.Done():
			log.Printf("Cleanup timeout exceeded after %v, forcing exit", shutdownTimeout)
		}

		if p != nil {
			p.Kill()
		}
	})
}

// getSignals returns the list of signals to handle based on the platform
func getSignals() []os.Signal {
	return getSignalsForPlatform()
}

// isSIGTSTP checks if the signal is SIGTSTP (only available on Unix)
func isSIGTSTP(sig os.Signal) bool {
	return isSIGTSTPForPlatform(sig)
}
