package tui

import (
	"context"
	"fmt"
	"strings"

	"bore-tui/internal/app"
	"bore-tui/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// createMode identifies which form variant is active.
const (
	createModeSelect   = 0 // choosing between modes
	createModeExisting = 1 // enter existing repo path
	createModeClone    = 2 // enter clone URL + destination
)

// CreateClusterScreen guides the user through creating a new cluster.
type CreateClusterScreen struct {
	app       *app.App
	styles    theme.Styles
	mode      int
	pathInput textinput.Model
	urlInput  textinput.Model
	destInput textinput.Model
	focus     int // which input has focus (0 or 1 for clone mode)
	err       error
	creating  bool
}

// NewCreateClusterScreen creates a new create-cluster screen.
func NewCreateClusterScreen(a *app.App, s theme.Styles) CreateClusterScreen {
	pi := textinput.New()
	pi.Placeholder = "/path/to/existing/repo"
	pi.CharLimit = 512
	pi.Width = 50

	ui := textinput.New()
	ui.Placeholder = "https://github.com/user/repo.git"
	ui.CharLimit = 512
	ui.Width = 50

	di := textinput.New()
	di.Placeholder = "/path/to/destination"
	di.CharLimit = 512
	di.Width = 50

	return CreateClusterScreen{
		app:       a,
		styles:    s,
		mode:      createModeSelect,
		pathInput: pi,
		urlInput:  ui,
		destInput: di,
	}
}

// reset clears all state so the screen is fresh on re-entry.
func (s *CreateClusterScreen) reset() {
	s.mode = createModeSelect
	s.pathInput.Reset()
	s.urlInput.Reset()
	s.destInput.Reset()
	s.focus = 0
	s.err = nil
	s.creating = false
}

// Update handles messages for the create-cluster screen.
func (s *CreateClusterScreen) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ErrorMsg:
		s.creating = false
		s.err = msg.Err
		return nil
	case tea.KeyMsg:
		km := DefaultKeyMap()

		// In select mode, choose between existing and clone.
		if s.mode == createModeSelect {
			switch {
			case msg.String() == "esc":
				return func() tea.Msg { return NavigateBackMsg{} }
			case key.Matches(msg, km.Up):
				if s.focus > 0 {
					s.focus--
				}
			case key.Matches(msg, km.Down):
				if s.focus < 1 {
					s.focus++
				}
			case key.Matches(msg, km.Enter):
				if s.focus == 0 {
					s.mode = createModeExisting
					s.pathInput.Focus()
					return textinput.Blink
				}
				s.mode = createModeClone
				s.focus = 0
				s.urlInput.Focus()
				return textinput.Blink
			}
			return nil
		}

		// In form modes, handle tab to switch focus and enter to submit.
		switch msg.String() {
		case "esc":
			s.mode = createModeSelect
			s.focus = 0
			s.pathInput.Blur()
			s.urlInput.Blur()
			s.destInput.Blur()
			s.err = nil
			return nil
		case "tab":
			return s.cycleInputFocus()
		case "enter":
			if !s.creating {
				return s.submit()
			}
			return nil
		}

		// Forward key events to the focused text input.
		return s.updateFocusedInput(msg)
	}

	return nil
}

// cycleInputFocus moves focus between inputs in clone mode.
func (s *CreateClusterScreen) cycleInputFocus() tea.Cmd {
	if s.mode == createModeClone {
		if s.focus == 0 {
			s.focus = 1
			s.urlInput.Blur()
			s.destInput.Focus()
		} else {
			s.focus = 0
			s.destInput.Blur()
			s.urlInput.Focus()
		}
		return textinput.Blink
	}
	return nil
}

// updateFocusedInput forwards key events to the active text input.
func (s *CreateClusterScreen) updateFocusedInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch s.mode {
	case createModeExisting:
		s.pathInput, cmd = s.pathInput.Update(msg)
	case createModeClone:
		if s.focus == 0 {
			s.urlInput, cmd = s.urlInput.Update(msg)
		} else {
			s.destInput, cmd = s.destInput.Update(msg)
		}
	}
	return cmd
}

// submit creates the cluster based on the current mode.
func (s *CreateClusterScreen) submit() tea.Cmd {
	switch s.mode {
	case createModeExisting:
		path := strings.TrimSpace(s.pathInput.Value())
		if path == "" {
			s.err = fmt.Errorf("path is required")
			return nil
		}
		s.creating = true
		s.err = nil
		a := s.app
		return func() tea.Msg {
			if err := a.InitCluster(context.Background(), path); err != nil {
				return ErrorMsg{Err: fmt.Errorf("init cluster: %w", err)}
			}
			return ClusterInitDoneMsg{}
		}

	case createModeClone:
		url := strings.TrimSpace(s.urlInput.Value())
		dest := strings.TrimSpace(s.destInput.Value())
		if url == "" {
			s.err = fmt.Errorf("clone URL is required")
			return nil
		}
		if dest == "" {
			s.err = fmt.Errorf("destination path is required")
			return nil
		}
		s.creating = true
		s.err = nil
		a := s.app
		return func() tea.Msg {
			if err := a.InitClusterFromClone(context.Background(), url, dest); err != nil {
				return ErrorMsg{Err: fmt.Errorf("clone and init: %w", err)}
			}
			return ClusterInitDoneMsg{}
		}
	}

	return nil
}

// View renders the create-cluster screen.
func (s *CreateClusterScreen) View(width, height int) string {
	var sections []string

	// Header.
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.ColorTextPrimary).
		Background(theme.ColorPrimary).
		Padding(0, 3).
		Render("Create Cluster")

	sections = append(sections, title, "")

	// Error display.
	if s.err != nil {
		errLine := lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true).
			Render(fmt.Sprintf("  Error: %s", s.err.Error()))
		sections = append(sections, errLine, "")
	}

	// Creating indicator.
	if s.creating {
		creating := lipgloss.NewStyle().
			Foreground(theme.ColorWarning).
			Italic(true).
			Render("  Creating cluster...")
		sections = append(sections, creating)
		return s.wrapContent(sections, width, height)
	}

	// Mode select.
	if s.mode == createModeSelect {
		prompt := lipgloss.NewStyle().
			Foreground(theme.ColorTextPrimary).
			Render("  How would you like to create a cluster?")
		sections = append(sections, prompt, "")

		options := []string{"Use existing repository path", "Clone from URL"}
		for i, opt := range options {
			if i == s.focus {
				btn := s.styles.ButtonFocused.Render(opt)
				sections = append(sections, "  "+btn)
			} else {
				btn := s.styles.Button.Render(opt)
				sections = append(sections, "  "+btn)
			}
		}

		sections = append(sections, "")
		hint := lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Italic(true).
			Render("  Use arrow keys to select, Enter to confirm, Esc to go back")
		sections = append(sections, hint)

		return s.wrapContent(sections, width, height)
	}

	// Existing repo form.
	if s.mode == createModeExisting {
		label := lipgloss.NewStyle().
			Foreground(theme.ColorTextPrimary).
			Bold(true).
			Render("  Repository Path")
		sections = append(sections, label, "")
		sections = append(sections, "  "+s.pathInput.View())
		sections = append(sections, "")

		hint := lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Italic(true).
			Render("  Enter to create, Esc to go back")
		sections = append(sections, hint)

		return s.wrapContent(sections, width, height)
	}

	// Clone form.
	if s.mode == createModeClone {
		urlLabel := lipgloss.NewStyle().
			Foreground(theme.ColorTextPrimary).
			Bold(true).
			Render("  Clone URL")

		destLabel := lipgloss.NewStyle().
			Foreground(theme.ColorTextPrimary).
			Bold(true).
			Render("  Destination Path")

		sections = append(sections, urlLabel, "")
		sections = append(sections, "  "+s.urlInput.View())
		sections = append(sections, "")
		sections = append(sections, destLabel, "")
		sections = append(sections, "  "+s.destInput.View())
		sections = append(sections, "")

		hint := lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Italic(true).
			Render("  Tab to switch fields, Enter to create, Esc to go back")
		sections = append(sections, hint)

		return s.wrapContent(sections, width, height)
	}

	return s.wrapContent(sections, width, height)
}

// wrapContent pads and positions the form content within the viewport.
func (s *CreateClusterScreen) wrapContent(sections []string, width, height int) string {
	content := strings.Join(sections, "\n")
	contentHeight := lipgloss.Height(content)

	padTop := 0
	if height > contentHeight {
		padTop = (height - contentHeight) / 3
	}

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		PaddingTop(padTop).
		PaddingLeft(4).
		Render(content)
}
