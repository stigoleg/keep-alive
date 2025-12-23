package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/stigoleg/keep-alive/internal/config"
	"github.com/stigoleg/keep-alive/internal/keepalive"
	"github.com/stigoleg/keep-alive/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	appVersion      = "1.4.6"
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
		log.Fatal(err)
	}

	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		log.Fatal(err)
	}
	logFile = f
	defer func() {
		if logFile != nil {
			logFile.Sync()
			logFile.Close()
		}
	}()

	var model ui.Model
	if cfg.Duration > 0 {
		model = ui.InitialModelWithDuration(cfg.Duration)
	} else {
		model = ui.InitialModel()
	}
	model.SetVersion(appVersion)
	model.SimulateActivity = cfg.SimulateActivity

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

	// Handle signals in a separate goroutine
	go func() {
		for sig := range sigChan {
			log.Printf("Received signal: %v", sig)

			// Handle SIGTSTP (Ctrl+Z) - prevent suspension and initiate shutdown
			if isSIGTSTP(sig) {
				log.Printf("SIGTSTP received: preventing suspension and initiating graceful shutdown")
			}

			executeCleanup(p)
		}
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
