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
	createModeExisting = 1 // browse for existing repo path
	createModeClone    = 2 // enter clone URL + browse destination
)

// createCloneFocus identifies which field has focus in clone mode.
const (
	cloneFocusURL    = 0
	cloneFocusPicker = 1
)

// CreateClusterScreen guides the user through creating a new cluster.
type CreateClusterScreen struct {
	app    *app.App
	styles theme.Styles
	mode   int
	focus  int // which item has focus (select mode: 0/1, clone mode: url=0, picker=1)

	// Clone mode fields.
	urlInput textinput.Model

	// Folder picker used in both existing and clone modes.
	picker       FolderPicker
	pickerActive bool   // true when folder picker is being displayed
	selectedPath string // path chosen by the folder picker

	err      error
	creating bool
	width    int
	height   int
}

// NewCreateClusterScreen creates a new create-cluster screen.
func NewCreateClusterScreen(a *app.App, s theme.Styles) CreateClusterScreen {
	ui := textinput.New()
	ui.Placeholder = "https://github.com/user/repo.git"
	ui.CharLimit = 512
	ui.Width = 50

	return CreateClusterScreen{
		app:    a,
		styles: s,
		mode:   createModeSelect,
		urlInput: ui,
		picker: NewFolderPicker(s),
	}
}

// reset clears all state so the screen is fresh on re-entry.
func (s *CreateClusterScreen) reset() {
	s.mode = createModeSelect
	s.urlInput.Reset()
	s.focus = 0
	s.err = nil
	s.creating = false
	s.pickerActive = false
	s.selectedPath = ""
	s.picker = NewFolderPicker(s.styles)
}

// Update handles messages for the create-cluster screen.
func (s *CreateClusterScreen) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ErrorMsg:
		s.creating = false
		s.err = msg.Err
		return nil

	case FolderSelectedMsg:
		s.selectedPath = msg.Path
		s.pickerActive = false
		if s.mode == createModeExisting {
			// Immediately submit with the selected path.
			return s.submitExisting(msg.Path)
		}
		// Clone mode: path selected for destination, return to url input.
		s.focus = cloneFocusURL
		return nil

	case FolderPickerCancelMsg:
		s.pickerActive = false
		if s.mode == createModeExisting {
			// Cancel returns to mode select.
			s.mode = createModeSelect
			s.focus = 0
			s.err = nil
		} else if s.mode == createModeClone {
			// Cancel returns focus to url input.
			s.focus = cloneFocusURL
			s.urlInput.Focus()
		}
		return nil

	case folderEntriesMsg:
		// Forward internal picker messages.
		if s.pickerActive {
			_, cmd := s.picker.Update(msg)
			return cmd
		}
		return nil

	case tea.KeyMsg:
		// If folder picker is active, delegate all key input to it.
		if s.pickerActive {
			_, cmd := s.picker.Update(msg)
			return cmd
		}

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
					s.pickerActive = true
					return s.picker.SetDirectory("")
				}
				s.mode = createModeClone
				s.focus = cloneFocusURL
				s.urlInput.Focus()
				return textinput.Blink
			}
			return nil
		}

		// Existing mode without active picker should not happen, but handle esc.
		if s.mode == createModeExisting {
			if msg.String() == "esc" {
				s.mode = createModeSelect
				s.focus = 0
				s.err = nil
				return nil
			}
			return nil
		}

		// Clone mode: handle tab to switch between url and picker, enter to submit.
		if s.mode == createModeClone {
			switch msg.String() {
			case "esc":
				s.mode = createModeSelect
				s.focus = 0
				s.urlInput.Blur()
				s.err = nil
				s.selectedPath = ""
				return nil
			case "tab":
				return s.cycleCloneFocus()
			case "enter":
				if !s.creating && s.focus == cloneFocusURL {
					return s.submitClone()
				}
				return nil
			}

			// Forward key events to the url input when it has focus.
			if s.focus == cloneFocusURL {
				var cmd tea.Cmd
				s.urlInput, cmd = s.urlInput.Update(msg)
				return cmd
			}
		}

		return nil

	case tea.MouseMsg:
		if s.pickerActive {
			_, cmd := s.picker.Update(msg)
			return cmd
		}

		if s.creating {
			return nil
		}
		if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
			return nil
		}

		// Compute vertical padding the same way wrapContent does.
		baseLines := 2 // title + blank
		if s.err != nil {
			baseLines += 2 // error + blank
		}

		if s.mode == createModeSelect {
			// prompt + blank + 2 options + blank + hint = 6 lines
			contentLines := baseLines + 6
			padTop := 0
			ch := s.height - 1
			if ch > contentLines {
				padTop = (ch - contentLines) / 3
			}
			y := msg.Y - padTop
			optionsStart := baseLines + 2
			if y >= optionsStart && y < optionsStart+2 {
				clicked := y - optionsStart
				s.focus = clicked
				if s.focus == 0 {
					s.mode = createModeExisting
					s.pickerActive = true
					return s.picker.SetDirectory("")
				}
				s.mode = createModeClone
				s.focus = cloneFocusURL
				s.urlInput.Focus()
				return textinput.Blink
			}
		}
	}

	return nil
}

// cycleCloneFocus moves focus between url input and folder picker in clone mode.
func (s *CreateClusterScreen) cycleCloneFocus() tea.Cmd {
	if s.focus == cloneFocusURL {
		s.focus = cloneFocusPicker
		s.urlInput.Blur()
		s.pickerActive = true
		if s.selectedPath == "" {
			return s.picker.SetDirectory("")
		}
		return s.picker.SetDirectory(s.selectedPath)
	}
	// Switching back from picker is handled by FolderSelectedMsg / FolderPickerCancelMsg.
	// But we also allow tab to exit the picker and return to url.
	s.focus = cloneFocusURL
	s.pickerActive = false
	s.urlInput.Focus()
	return textinput.Blink
}

// submitExisting creates the cluster from an existing repo path.
func (s *CreateClusterScreen) submitExisting(path string) tea.Cmd {
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
}

// submitClone creates the cluster by cloning from a URL.
func (s *CreateClusterScreen) submitClone() tea.Cmd {
	url := strings.TrimSpace(s.urlInput.Value())
	dest := s.selectedPath
	if url == "" {
		s.err = fmt.Errorf("clone URL is required")
		return nil
	}
	if dest == "" {
		s.err = fmt.Errorf("destination path is required — press Tab to browse")
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

// View renders the create-cluster screen.
func (s *CreateClusterScreen) View(width, height int) string {
	s.width = width
	s.height = height
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

	// Existing repo mode — show folder picker.
	if s.mode == createModeExisting {
		label := lipgloss.NewStyle().
			Foreground(theme.ColorTextPrimary).
			Bold(true).
			Render("  Browse to Repository")
		sections = append(sections, label, "")

		// Allocate remaining height to the folder picker.
		headerHeight := len(sections)
		pickerHeight := height - headerHeight - 2 // leave room for padding
		if pickerHeight < 6 {
			pickerHeight = 6
		}
		pickerWidth := width - 8 // account for wrapContent left padding
		if pickerWidth < 20 {
			pickerWidth = 20
		}
		sections = append(sections, s.picker.View(pickerWidth, pickerHeight))

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

		if s.pickerActive {
			// Show folder picker inline.
			headerHeight := len(sections)
			pickerHeight := height - headerHeight - 2
			if pickerHeight < 6 {
				pickerHeight = 6
			}
			pickerWidth := width - 8
			if pickerWidth < 20 {
				pickerWidth = 20
			}
			sections = append(sections, s.picker.View(pickerWidth, pickerHeight))
		} else if s.selectedPath != "" {
			// Show the selected destination path.
			pathDisplay := lipgloss.NewStyle().
				Foreground(theme.ColorSuccess).
				Bold(true).
				Render("  " + s.selectedPath)
			sections = append(sections, pathDisplay)
			sections = append(sections, "")

			hint := lipgloss.NewStyle().
				Foreground(theme.ColorTextSecondary).
				Italic(true).
				Render("  Tab to change destination, Enter to create, Esc to go back")
			sections = append(sections, hint)
		} else {
			noPath := lipgloss.NewStyle().
				Foreground(theme.ColorTextSecondary).
				Italic(true).
				Render("  Press Tab to browse for destination folder")
			sections = append(sections, noPath)
			sections = append(sections, "")

			hint := lipgloss.NewStyle().
				Foreground(theme.ColorTextSecondary).
				Italic(true).
				Render("  Tab to browse destination, Esc to go back")
			sections = append(sections, hint)
		}

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
