package main

import (
	"log"
	"os"

	"keepalive/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	p := tea.NewProgram(
		ui.InitialModel(),
		tea.WithAltScreen(),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
