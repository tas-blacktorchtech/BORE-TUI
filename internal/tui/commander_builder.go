package tui

import (
	"context"
	"fmt"
	"strings"

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

// memoriesLoadedMsg is sent when the memory list has been fetched from the DB.
type memoriesLoadedMsg struct {
	Memories []db.CommanderMemory
}

// memorySavedMsg is sent after a memory entry has been persisted.
type memorySavedMsg struct{}

// memoryDeletedMsg is sent after a memory entry has been deleted.
type memoryDeletedMsg struct{ Key string }

// ---------------------------------------------------------------------------
// CommanderBuilderScreen
// ---------------------------------------------------------------------------

// CommanderBuilderScreen provides a list + editor interface for the
// commander's key-value "brain" memory stored in the database.
type CommanderBuilderScreen struct {
	app    *app.App
	styles theme.Styles

	memories []db.CommanderMemory
	cursor   int

	editing    bool
	creating   bool
	deleting   bool
	deleteKey  string
	keyInput   textinput.Model
	valueInput textinput.Model
	focus      int // 0=key, 1=value

	width, height int
	loaded        bool
	statusMsg     string
}

// NewCommanderBuilderScreen creates a CommanderBuilderScreen ready for use.
func NewCommanderBuilderScreen(a *app.App, s theme.Styles) CommanderBuilderScreen {
	ki := textinput.New()
	ki.Placeholder = "Key"
	ki.CharLimit = 128
	ki.Width = 40

	vi := textinput.New()
	vi.Placeholder = "Value"
	vi.CharLimit = 1024
	vi.Width = 60

	return CommanderBuilderScreen{
		app:        a,
		styles:     s,
		keyInput:   ki,
		valueInput: vi,
	}
}

// Init returns the command to load all memory entries.
func (c *CommanderBuilderScreen) Init() tea.Cmd {
	return c.loadMemories()
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles messages and key events for the commander builder.
func (c *CommanderBuilderScreen) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height

	case memoriesLoadedMsg:
		c.memories = msg.Memories
		c.loaded = true
		if c.cursor >= len(c.memories) && len(c.memories) > 0 {
			c.cursor = len(c.memories) - 1
		}

	case memorySavedMsg:
		c.editing = false
		c.creating = false
		c.statusMsg = "Saved"
		cmds = append(cmds, c.loadMemories())

	case memoryDeletedMsg:
		c.deleting = false
		c.statusMsg = fmt.Sprintf("Deleted key %q", msg.Key)
		cmds = append(cmds, c.loadMemories())

	case ErrorMsg:
		c.statusMsg = fmt.Sprintf("Error: %v", msg.Err)

	case tea.KeyMsg:
		// Delete confirmation mode.
		if c.deleting {
			switch msg.String() {
			case "y", "Y":
				cmds = append(cmds, c.deleteMemory(c.deleteKey))
			case "n", "N", "esc":
				c.deleting = false
			}
			return tea.Batch(cmds...)
		}

		// Editing or creating mode — delegate to form handler.
		if c.editing || c.creating {
			return c.updateForm(msg)
		}

		// List mode.
		switch msg.String() {
		case "up", "k":
			if len(c.memories) > 0 {
				c.cursor = (c.cursor - 1 + len(c.memories)) % len(c.memories)
			}
		case "down", "j":
			if len(c.memories) > 0 {
				c.cursor = (c.cursor + 1) % len(c.memories)
			}
		case "a":
			c.creating = true
			c.keyInput.SetValue("")
			c.valueInput.SetValue("")
			c.focus = 0
			c.keyInput.Focus()
			c.valueInput.Blur()
			c.statusMsg = ""
		case "enter", "e":
			if c.cursor < len(c.memories) {
				m := c.memories[c.cursor]
				c.editing = true
				c.keyInput.SetValue(m.Key)
				c.valueInput.SetValue(m.Value)
				c.focus = 1
				c.keyInput.Blur()
				c.valueInput.Focus()
				c.statusMsg = ""
			}
		case "d":
			if c.cursor < len(c.memories) {
				c.deleting = true
				c.deleteKey = c.memories[c.cursor].Key
			}
		case "esc":
			return func() tea.Msg { return NavigateBackMsg{} }
		case "r":
			cmds = append(cmds, c.loadMemories())
		}
	}

	return tea.Batch(cmds...)
}

// updateForm handles key events when in the editing or creating state.
func (c *CommanderBuilderScreen) updateForm(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "tab", "shift+tab":
		if c.focus == 0 {
			c.focus = 1
			c.keyInput.Blur()
			c.valueInput.Focus()
		} else {
			c.focus = 0
			c.valueInput.Blur()
			c.keyInput.Focus()
		}
		return nil

	case "esc":
		c.editing = false
		c.creating = false
		return nil

	case "enter":
		key := strings.TrimSpace(c.keyInput.Value())
		value := strings.TrimSpace(c.valueInput.Value())
		if key == "" {
			c.statusMsg = "Key cannot be empty"
			return nil
		}
		return c.saveMemory(key, value)
	}

	// Delegate to the focused input.
	var cmd tea.Cmd
	if c.focus == 0 {
		c.keyInput, cmd = c.keyInput.Update(msg)
	} else {
		c.valueInput, cmd = c.valueInput.Update(msg)
	}
	return cmd
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the commander builder screen.
func (c *CommanderBuilderScreen) View(width, height int) string {
	c.width = width
	c.height = height
	if width == 0 {
		return ""
	}

	header := c.styles.Header.Width(width).Render(" Commander Brain Builder ")

	var body string
	if c.deleting {
		body = c.renderDeleteConfirmation()
	} else if c.editing || c.creating {
		body = c.renderForm()
	} else {
		body = c.renderList()
	}

	// Status line.
	statusLine := ""
	if c.statusMsg != "" {
		statusLine = lipgloss.NewStyle().
			Foreground(theme.ColorWarning).
			PaddingLeft(2).
			Render(c.statusMsg)
	}

	// Help bar.
	var helpText string
	if c.editing || c.creating {
		helpText = " [tab] switch field  [enter] save  [esc] cancel "
	} else {
		helpText = " [a] add  [e/enter] edit  [d] delete  [r] refresh  [esc] back "
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

func (c *CommanderBuilderScreen) renderList() string {
	if !c.loaded {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(4).
			Render("Loading...")
	}

	if len(c.memories) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(4).
			Render("No brain entries defined. Press 'a' to add one.")
	}

	maxLines := c.height - 8
	if maxLines < 1 {
		maxLines = 1
	}

	var lines []string
	for i, m := range c.memories {
		if i >= maxLines {
			break
		}
		keyStr := dashTruncate(m.Key, 24)
		valStr := dashTruncate(m.Value, 50)
		line := fmt.Sprintf("%-24s  %s", keyStr, valStr)

		if i == c.cursor {
			lines = append(lines, c.styles.ListItemSelected.Render(line))
		} else {
			lines = append(lines, c.styles.ListItem.Render(line))
		}
	}

	countLabel := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(2).
		Render(fmt.Sprintf("%d entries", len(c.memories)))

	return lipgloss.JoinVertical(lipgloss.Left,
		countLabel,
		"",
		lipgloss.JoinVertical(lipgloss.Left, lines...),
	)
}

func (c *CommanderBuilderScreen) renderForm() string {
	title := "Edit Memory Entry"
	if c.creating {
		title = "New Memory Entry"
	}
	titleLine := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		Bold(true).
		PaddingLeft(4).
		Render(title)

	keyStyle := c.styles.Input
	valStyle := c.styles.Input
	if c.focus == 0 {
		keyStyle = c.styles.InputFocused
	} else {
		valStyle = c.styles.InputFocused
	}

	keyLabel := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(4).
		Render("Key:")
	keyField := keyStyle.Render(c.keyInput.View())

	valLabel := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(4).
		Render("Value:")
	valField := valStyle.Render(c.valueInput.View())

	return lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		"",
		keyLabel,
		"    "+keyField,
		"",
		valLabel,
		"    "+valField,
	)
}

func (c *CommanderBuilderScreen) renderDeleteConfirmation() string {
	return lipgloss.NewStyle().
		Foreground(theme.ColorAccent).
		Bold(true).
		PaddingLeft(4).
		Render(fmt.Sprintf("Delete key %q? (y/n)", c.deleteKey))
}

// ---------------------------------------------------------------------------
// Data commands
// ---------------------------------------------------------------------------

func (c *CommanderBuilderScreen) loadMemories() tea.Cmd {
	a := c.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return memoriesLoadedMsg{}
		}
		memories, err := a.DB().GetAllMemory(context.Background(), cluster.ID)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return memoriesLoadedMsg{Memories: memories}
	}
}

func (c *CommanderBuilderScreen) saveMemory(key, value string) tea.Cmd {
	a := c.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return ErrorMsg{Err: fmt.Errorf("no cluster open")}
		}
		err := a.DB().SetMemory(context.Background(), cluster.ID, key, value)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return memorySavedMsg{}
	}
}

func (c *CommanderBuilderScreen) deleteMemory(key string) tea.Cmd {
	a := c.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return ErrorMsg{Err: fmt.Errorf("no cluster open")}
		}
		err := a.DB().DeleteMemory(context.Background(), cluster.ID, key)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return memoryDeletedMsg{Key: key}
	}
}
