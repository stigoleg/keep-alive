package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/stigoleg/keep-alive/internal/config"
	"github.com/stigoleg/keep-alive/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

const appVersion = "1.3.5"

func main() {
	cfg, err := config.ParseFlags(appVersion)
	if err != nil {
		log.Fatal(err)
	}

	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var model ui.Model
	if cfg.Duration > 0 {
		model = ui.InitialModelWithDuration(cfg.Duration)
	} else {
		model = ui.InitialModel()
	}
	model.SetVersion(appVersion)

	// Create program with signal handling
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithoutSignalHandler(),
	)

	// Handle signals in a separate goroutine
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v", sig)
		if model.KeepAlive != nil {
			if err := model.KeepAlive.Stop(); err != nil {
				log.Printf("Error stopping keep-alive: %v", err)
			}
		}
		p.Kill()
	}()

	if _, err := p.Run(); err != nil {
		log.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
