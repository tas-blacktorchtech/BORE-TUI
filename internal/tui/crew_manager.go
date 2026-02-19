package tui

import (
	"context"
	"fmt"

	"bore-tui/internal/app"
	"bore-tui/internal/db"
	"bore-tui/internal/theme"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Internal message types
// ---------------------------------------------------------------------------

// crewSavedMsg is sent after a crew is created or updated.
type crewSavedMsg struct{}

// crewDeletedMsg is sent after a crew is deleted.
type crewDeletedMsg struct{ Name string }

// ---------------------------------------------------------------------------
// CrewManagerScreen
// ---------------------------------------------------------------------------

const (
	crewModeList   = 0
	crewModeCreate = 1
	crewModeEdit   = 2
	crewModeDelete = 3
)

// CrewManagerScreen provides CRUD operations for crews within the current
// cluster. It shows a list view and a multi-field form for create/edit.
type CrewManagerScreen struct {
	app    *app.App
	styles theme.Styles

	crews  []db.Crew
	cursor int
	mode   int // crewModeList, crewModeCreate, crewModeEdit, crewModeDelete

	nameInput        textinput.Model
	objectiveInput   textinput.Model
	constraintsInput textinput.Model
	commandsInput    textinput.Model
	pathsInput       textinput.Model
	formFocus        int // 0..4

	editingCrew *db.Crew // non-nil when editing an existing crew
	deleteName  string

	width, height int
	loaded        bool
	statusMsg     string
}

// NewCrewManagerScreen creates a CrewManagerScreen ready for use.
func NewCrewManagerScreen(a *app.App, s theme.Styles) CrewManagerScreen {
	makeInput := func(placeholder string, charLimit int) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.CharLimit = charLimit
		ti.Width = 60
		return ti
	}

	return CrewManagerScreen{
		app:              a,
		styles:           s,
		nameInput:        makeInput("Crew name", 64),
		objectiveInput:   makeInput("Objective", 256),
		constraintsInput: makeInput("Constraints (comma-separated)", 512),
		commandsInput:    makeInput("Allowed commands (comma-separated)", 512),
		pathsInput:       makeInput("Ownership paths (comma-separated)", 512),
	}
}

// Init returns the command to load all crews.
func (c *CrewManagerScreen) Init() tea.Cmd {
	return c.loadCrews()
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles messages and key events for the crew manager.
func (c *CrewManagerScreen) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height

	case CrewsLoadedMsg:
		c.crews = msg.Crews
		c.loaded = true
		if c.cursor >= len(c.crews) && len(c.crews) > 0 {
			c.cursor = len(c.crews) - 1
		}

	case crewSavedMsg:
		c.mode = crewModeList
		c.editingCrew = nil
		c.statusMsg = "Crew saved"
		cmds = append(cmds, c.loadCrews())

	case crewDeletedMsg:
		c.mode = crewModeList
		c.statusMsg = fmt.Sprintf("Deleted %q", msg.Name)
		cmds = append(cmds, c.loadCrews())

	case ErrorMsg:
		c.statusMsg = fmt.Sprintf("Error: %v", msg.Err)

	case tea.KeyMsg:
		// Delete confirmation.
		if c.mode == crewModeDelete {
			switch msg.String() {
			case "y", "Y":
				if c.cursor < len(c.crews) {
					crew := c.crews[c.cursor]
					cmds = append(cmds, c.deleteCrew(crew.ID, crew.Name))
				}
			case "n", "N", "esc":
				c.mode = crewModeList
			}
			return tea.Batch(cmds...)
		}

		// Form mode.
		if c.mode == crewModeCreate || c.mode == crewModeEdit {
			return c.updateForm(msg)
		}

		// List mode.
		switch msg.String() {
		case "up", "k":
			if len(c.crews) > 0 {
				c.cursor = (c.cursor - 1 + len(c.crews)) % len(c.crews)
			}
		case "down", "j":
			if len(c.crews) > 0 {
				c.cursor = (c.cursor + 1) % len(c.crews)
			}
		case "c":
			c.mode = crewModeCreate
			c.editingCrew = nil
			c.clearForm()
			c.formFocus = 0
			c.focusField(0)
			c.statusMsg = ""
		case "enter", "e":
			if c.cursor < len(c.crews) {
				crew := c.crews[c.cursor]
				c.mode = crewModeEdit
				c.editingCrew = &crew
				c.nameInput.SetValue(crew.Name)
				c.objectiveInput.SetValue(crew.Objective)
				c.constraintsInput.SetValue(crew.Constraints)
				c.commandsInput.SetValue(crew.AllowedCommands)
				c.pathsInput.SetValue(crew.OwnershipPaths)
				c.formFocus = 0
				c.focusField(0)
				c.statusMsg = ""
			}
		case "d":
			if c.cursor < len(c.crews) {
				c.mode = crewModeDelete
				c.deleteName = c.crews[c.cursor].Name
			}
		case "esc":
			return func() tea.Msg { return NavigateBackMsg{} }
		case "r":
			cmds = append(cmds, c.loadCrews())
		}
	}

	return tea.Batch(cmds...)
}

// updateForm handles key events within the create/edit form.
func (c *CrewManagerScreen) updateForm(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "tab":
		c.formFocus = (c.formFocus + 1) % 5
		c.focusField(c.formFocus)
		return nil
	case "shift+tab":
		c.formFocus = (c.formFocus + 4) % 5
		c.focusField(c.formFocus)
		return nil
	case "esc":
		c.mode = crewModeList
		c.editingCrew = nil
		return nil
	case "ctrl+s":
		return c.saveCrew()
	}

	// Delegate to the focused input.
	var cmd tea.Cmd
	switch c.formFocus {
	case 0:
		c.nameInput, cmd = c.nameInput.Update(msg)
	case 1:
		c.objectiveInput, cmd = c.objectiveInput.Update(msg)
	case 2:
		c.constraintsInput, cmd = c.constraintsInput.Update(msg)
	case 3:
		c.commandsInput, cmd = c.commandsInput.Update(msg)
	case 4:
		c.pathsInput, cmd = c.pathsInput.Update(msg)
	}
	return cmd
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the crew manager screen.
func (c *CrewManagerScreen) View(width, height int) string {
	c.width = width
	c.height = height
	if width == 0 {
		return ""
	}

	header := c.styles.Header.Width(width).Render(" Crew Manager ")

	var body string
	switch c.mode {
	case crewModeDelete:
		body = c.renderDeleteConfirmation()
	case crewModeCreate, crewModeEdit:
		body = c.renderForm()
	default:
		body = c.renderList()
	}

	statusLine := ""
	if c.statusMsg != "" {
		statusLine = lipgloss.NewStyle().
			Foreground(theme.ColorWarning).
			PaddingLeft(2).
			Render(c.statusMsg)
	}

	var helpText string
	switch c.mode {
	case crewModeCreate, crewModeEdit:
		helpText = " [tab] next field  [shift+tab] prev  [ctrl+s] save  [esc] cancel "
	default:
		helpText = " [c] create  [e/enter] edit  [d] delete  [r] refresh  [esc] back "
	}
	helpBar := c.styles.CommandBar.Width(width).Render(helpText)

	parts := []string{header, "", body}
	if statusLine != "" {
		parts = append(parts, "", statusLine)
	}
	parts = append(parts, helpBar)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ---------------------------------------------------------------------------
// Rendering helpers
// ---------------------------------------------------------------------------

func (c *CrewManagerScreen) renderList() string {
	if !c.loaded {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(4).
			Render("Loading...")
	}

	if len(c.crews) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(4).
			Render("No crews defined. Press 'c' to create one.")
	}

	maxLines := c.height - 8
	if maxLines < 1 {
		maxLines = 1
	}

	var lines []string
	for i, crew := range c.crews {
		if i >= maxLines {
			break
		}
		name := dashTruncate(crew.Name, 20)
		obj := dashTruncate(crew.Objective, 40)
		line := fmt.Sprintf("%-20s  %s", name, obj)

		if i == c.cursor {
			lines = append(lines, c.styles.ListItemSelected.Render(line))
		} else {
			lines = append(lines, c.styles.ListItem.Render(line))
		}
	}

	countLabel := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(2).
		Render(fmt.Sprintf("%d crews", len(c.crews)))

	return lipgloss.JoinVertical(lipgloss.Left,
		countLabel,
		"",
		lipgloss.JoinVertical(lipgloss.Left, lines...),
	)
}

func (c *CrewManagerScreen) renderForm() string {
	title := "Create Crew"
	if c.mode == crewModeEdit {
		title = "Edit Crew"
	}
	titleLine := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		Bold(true).
		PaddingLeft(4).
		Render(title)

	fields := []struct {
		label string
		input textinput.Model
	}{
		{"Name:", c.nameInput},
		{"Objective:", c.objectiveInput},
		{"Constraints:", c.constraintsInput},
		{"Allowed Commands:", c.commandsInput},
		{"Ownership Paths:", c.pathsInput},
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(4)

	var parts []string
	parts = append(parts, titleLine, "")

	for i, f := range fields {
		style := c.styles.Input
		if i == c.formFocus {
			style = c.styles.InputFocused
		}
		parts = append(parts,
			labelStyle.Render(f.label),
			"    "+style.Render(f.input.View()),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (c *CrewManagerScreen) renderDeleteConfirmation() string {
	return lipgloss.NewStyle().
		Foreground(theme.ColorAccent).
		Bold(true).
		PaddingLeft(4).
		Render(fmt.Sprintf("Delete crew %q? (y/n)", c.deleteName))
}

// ---------------------------------------------------------------------------
// Form helpers
// ---------------------------------------------------------------------------

func (c *CrewManagerScreen) clearForm() {
	c.nameInput.SetValue("")
	c.objectiveInput.SetValue("")
	c.constraintsInput.SetValue("")
	c.commandsInput.SetValue("")
	c.pathsInput.SetValue("")
}

func (c *CrewManagerScreen) focusField(idx int) {
	c.nameInput.Blur()
	c.objectiveInput.Blur()
	c.constraintsInput.Blur()
	c.commandsInput.Blur()
	c.pathsInput.Blur()

	switch idx {
	case 0:
		c.nameInput.Focus()
	case 1:
		c.objectiveInput.Focus()
	case 2:
		c.constraintsInput.Focus()
	case 3:
		c.commandsInput.Focus()
	case 4:
		c.pathsInput.Focus()
	}
}

// ---------------------------------------------------------------------------
// Data commands
// ---------------------------------------------------------------------------

func (c *CrewManagerScreen) loadCrews() tea.Cmd {
	a := c.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return CrewsLoadedMsg{}
		}
		crews, err := a.DB().ListCrews(context.Background(), cluster.ID)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return CrewsLoadedMsg{Crews: crews}
	}
}

func (c *CrewManagerScreen) saveCrew() tea.Cmd {
	name := c.nameInput.Value()
	objective := c.objectiveInput.Value()
	constraints := c.constraintsInput.Value()
	commands := c.commandsInput.Value()
	paths := c.pathsInput.Value()
	editing := c.editingCrew
	a := c.app

	if name == "" {
		return func() tea.Msg {
			return ErrorMsg{Err: fmt.Errorf("crew name is required")}
		}
	}

	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return ErrorMsg{Err: fmt.Errorf("no cluster open")}
		}
		ctx := context.Background()

		if editing != nil {
			crew := *editing
			crew.Name = name
			crew.Objective = objective
			crew.Constraints = constraints
			crew.AllowedCommands = commands
			crew.OwnershipPaths = paths
			if err := a.DB().UpdateCrew(ctx, &crew); err != nil {
				return ErrorMsg{Err: err}
			}
		} else {
			_, err := a.DB().CreateCrew(ctx, cluster.ID, name, objective, constraints, commands, paths)
			if err != nil {
				return ErrorMsg{Err: err}
			}
		}
		return crewSavedMsg{}
	}
}

func (c *CrewManagerScreen) deleteCrew(id int64, name string) tea.Cmd {
	a := c.app
	return func() tea.Msg {
		err := a.DB().DeleteCrew(context.Background(), id)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return crewDeletedMsg{Name: name}
	}
}
