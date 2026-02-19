package tui

import (
	"bore-tui/internal/app"
	"bore-tui/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommanderDashboardScreen is a simple menu screen with two options:
// Commander Brain (edit brain/memory) and Commander Chat (freeform Q&A).
type CommanderDashboardScreen struct {
	app    *app.App
	styles theme.Styles

	cursor        int // 0=Brain, 1=Chat
	width, height int
}

// NewCommanderDashboardScreen creates a CommanderDashboardScreen ready for use.
func NewCommanderDashboardScreen(a *app.App, s theme.Styles) CommanderDashboardScreen {
	return CommanderDashboardScreen{app: a, styles: s}
}

// Init is a no-op — no async work needed on entry.
func (c *CommanderDashboardScreen) Init() tea.Cmd { return nil }

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles input for the commander dashboard menu.
func (c *CommanderDashboardScreen) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if c.cursor > 0 {
				c.cursor--
			}
		case "down", "j":
			if c.cursor < 1 {
				c.cursor++
			}
		case "enter", " ":
			return c.activate()
		case "esc":
			return func() tea.Msg { return NavigateBackMsg{} }
		}

	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			// Each button row is roughly 3 lines tall; first starts at ~centerY-3.
			centerY := c.height / 2
			brainY := centerY - 4
			chatY := centerY + 1
			if msg.Y >= brainY && msg.Y < brainY+3 {
				c.cursor = 0
				return c.activate()
			}
			if msg.Y >= chatY && msg.Y < chatY+3 {
				c.cursor = 1
				return c.activate()
			}
		}
	}

	return nil
}

func (c *CommanderDashboardScreen) activate() tea.Cmd {
	switch c.cursor {
	case 0:
		return func() tea.Msg { return NavigateMsg{Screen: ScreenCommanderBuilder} }
	case 1:
		return func() tea.Msg { return NavigateMsg{Screen: ScreenCommanderChat} }
	}
	return nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the two-button commander dashboard.
func (c *CommanderDashboardScreen) View(width, height int) string {
	c.width = width
	c.height = height
	if width == 0 {
		return ""
	}

	title := c.styles.Header.Render(" Commander Dashboard ")
	subtitle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Render("Choose a Commander tool")

	brainBtn := c.renderButton(0, "Commander Brain",
		"Edit the Commander's persistent memory & knowledge base")
	chatBtn := c.renderButton(1, "Commander Chat",
		"Ask the Commander questions about the project & task history")

	buttons := lipgloss.JoinVertical(lipgloss.Left,
		brainBtn,
		"",
		chatBtn,
	)

	center := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(buttons)

	hint := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Width(width).
		Align(lipgloss.Center).
		Render("↑/↓ navigate  enter select  esc back")

	content := lipgloss.JoinVertical(lipgloss.Left,
		"",
		title,
		"",
		subtitle,
		"",
		"",
		center,
		"",
		hint,
	)

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Render(content)
}

func (c *CommanderDashboardScreen) renderButton(idx int, label, desc string) string {
	btnW := 54
	if c.width > 0 && c.width/2 > btnW {
		btnW = c.width / 2
	}
	if btnW > 70 {
		btnW = 70
	}

	labelStyle := lipgloss.NewStyle().Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(theme.ColorTextSecondary)

	inner := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render(label),
		descStyle.Render(desc),
	)

	var box lipgloss.Style
	if idx == c.cursor {
		box = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.ColorPrimary).
			Foreground(theme.ColorPrimary).
			Padding(0, 2).
			Width(btnW)
	} else {
		box = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.ColorBorderSoft).
			Foreground(theme.ColorTextPrimary).
			Padding(0, 2).
			Width(btnW)
	}

	return lipgloss.NewStyle().
		Width(c.width).
		Align(lipgloss.Center).
		Render(box.Render(inner))
}
