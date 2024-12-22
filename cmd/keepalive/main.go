package main

import (
	"log"

	"github.com/stigoleg/keep-alive/internal/config"
	"github.com/stigoleg/keep-alive/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

const appVersion = "1.1.0"

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

	var model ui.Model
	if cfg.Duration > 0 {
		model = ui.InitialModelWithDuration(cfg.Duration)
		p := tea.NewProgram(
			model,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)
		if _, err := p.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}

	model = ui.InitialModel()
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
