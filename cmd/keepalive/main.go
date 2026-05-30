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
	appVersion      = "1.5.2"
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
		fmt.Fprintln(os.Stderr, err)
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
	if cfg.BatteryThreshold > 0 {
		status, err := platform.GetBatteryStatus()
		if err != nil {
			fmt.Fprintf(os.Stderr, "battery status unavailable: %v\n", err)
			os.Exit(1)
		}
		if status.Percentage <= cfg.BatteryThreshold {
			fmt.Fprintf(os.Stderr, "battery threshold must be below current battery percentage (current: %d%%, threshold: %d%%)\n", status.Percentage, cfg.BatteryThreshold)
			os.Exit(1)
		}
		model = ui.InitialModelWithBattery(cfg.BatteryThreshold, status, cfg.SimulateActivity)
	} else if cfg.Duration > 0 {
		model = ui.InitialModelWithDuration(cfg.Duration, cfg.SimulateActivity)
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

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signals := getSignals()
	signal.Notify(sigChan, signals...)

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
