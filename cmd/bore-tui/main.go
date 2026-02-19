package main

import (
	"fmt"
	"os"

	"bore-tui/internal/app"
	"bore-tui/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	a := app.New()
	defer a.Close()

	model := tui.NewModel(a)

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("bore-tui: %w", err)
	}

	return nil
}
