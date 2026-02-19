package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keybindings used across the TUI.
type KeyMap struct {
	Quit      key.Binding
	Back      key.Binding
	Help      key.Binding
	Tab       key.Binding
	Enter     key.Binding
	NewTask   key.Binding
	NewCrew   key.Binding
	NewThread key.Binding
	Refresh   key.Binding
	Up        key.Binding
	Down      key.Binding
}

// DefaultKeyMap returns the standard set of keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q/ctrl+c", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "cycle panes"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		NewTask: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new task"),
		),
		NewCrew: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "new crew"),
		),
		NewThread: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "new thread"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("up/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("down/j", "move down"),
		),
	}
}
