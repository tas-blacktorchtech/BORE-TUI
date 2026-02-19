package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bore-tui/internal/agents"
	"bore-tui/internal/app"
	"bore-tui/internal/theme"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Internal messages
// ---------------------------------------------------------------------------

// chatResponseMsg carries a completed Commander response.
type chatResponseMsg struct{ content string }

// chatErrMsg carries an error from a chat invocation.
type chatErrMsg struct{ err error }

// ---------------------------------------------------------------------------
// CommanderChatScreen
// ---------------------------------------------------------------------------

// CommanderChatScreen is a persistent conversational chat interface with the
// Commander agent. It maintains full conversation history for the session and
// rebuilds the prompt on each turn so the Commander has full context.
type CommanderChatScreen struct {
	app    *app.App
	styles theme.Styles

	// Conversation state — persists across navigations (stored on Model).
	messages []agents.ChatMessage

	// UI components
	viewport viewport.Model
	input    textarea.Model

	// State
	thinking   bool
	err        error
	spinnerIdx int

	width, height int
}

var chatSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewCommanderChatScreen creates a CommanderChatScreen ready for use.
func NewCommanderChatScreen(a *app.App, s theme.Styles) CommanderChatScreen {
	ta := textarea.New()
	ta.Placeholder = "Ask Commander anything about the project..."
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.CharLimit = 4000

	vp := viewport.New(0, 0)

	return CommanderChatScreen{
		app:      a,
		styles:   s,
		input:    ta,
		viewport: vp,
	}
}

// Init is called on navigation. It focuses the input and does not clear history.
func (c *CommanderChatScreen) Init() tea.Cmd {
	c.err = nil
	c.input.Focus()
	// Re-render history in case dimensions changed.
	c.refreshViewport()
	return nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles messages and key events for the Commander chat screen.
func (c *CommanderChatScreen) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.resize()

	case TickMsg:
		if c.thinking {
			c.spinnerIdx = (c.spinnerIdx + 1) % len(chatSpinnerFrames)
		}

	case chatResponseMsg:
		c.thinking = false
		c.messages = append(c.messages, agents.ChatMessage{
			Role:    "commander",
			Content: msg.content,
		})
		c.refreshViewport()
		c.viewport.GotoBottom()
		c.input.Focus()

	case chatErrMsg:
		c.thinking = false
		c.err = msg.err
		c.input.Focus()

	case tea.KeyMsg:
		if c.thinking {
			// Block input while waiting for a response.
			return nil
		}

		switch msg.String() {
		case "esc":
			return func() tea.Msg { return NavigateBackMsg{} }

		case "ctrl+l":
			// Clear chat history.
			c.messages = nil
			c.refreshViewport()

		case "enter":
			// Send message.
			text := strings.TrimSpace(c.input.Value())
			if text == "" {
				return nil
			}
			c.input.Reset()
			c.input.Blur()
			return c.sendMessage(text)

		default:
			// Delegate other keys to textarea.
			var taCmd tea.Cmd
			c.input, taCmd = c.input.Update(msg)
			if taCmd != nil {
				cmds = append(cmds, taCmd)
			}
		}

	case tea.MouseMsg:
		switch {
		case msg.Button == tea.MouseButtonWheelUp:
			c.viewport.LineUp(3)
		case msg.Button == tea.MouseButtonWheelDown:
			c.viewport.LineDown(3)
		}
	}

	return tea.Batch(cmds...)
}

// sendMessage appends the user message, triggers a Commander response, and
// returns a tea.Cmd that runs Claude CLI asynchronously.
func (c *CommanderChatScreen) sendMessage(text string) tea.Cmd {
	c.messages = append(c.messages, agents.ChatMessage{
		Role:    "user",
		Content: text,
	})
	c.thinking = true
	c.err = nil
	c.refreshViewport()
	c.viewport.GotoBottom()

	// Snapshot history and app reference for the goroutine.
	history := make([]agents.ChatMessage, len(c.messages)-1) // all but the one just appended
	copy(history, c.messages[:len(c.messages)-1])
	a := c.app

	return func() tea.Msg {
		cmdCtx, err := buildCommanderContext(context.Background(), a)
		if err != nil {
			return chatErrMsg{err: fmt.Errorf("commander chat: context: %w", err)}
		}

		systemPrompt := agents.BuildCommanderChatSystemPrompt(cmdCtx)
		userMsg := agents.BuildCommanderChatMessage(history, text)

		// Combine system prompt + user message into a single stdin prompt.
		fullPrompt := systemPrompt + "\n\n---\n\n" + userMsg

		repo := a.Repo()
		workDir := "."
		if repo != nil && repo.Path != "" {
			workDir = repo.Path
		}

		result := a.Runner().Run(
			context.Background(),
			workDir,
			fullPrompt,
			nil,
			nil,
			nil,
		)

		if result.Err != nil {
			return chatErrMsg{err: fmt.Errorf("commander chat: %w", result.Err)}
		}

		response := strings.TrimSpace(result.Stdout)
		if response == "" {
			response = "(no response)"
		}
		return chatResponseMsg{content: response}
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the Commander chat screen.
func (c *CommanderChatScreen) View(width, height int) string {
	c.width = width
	c.height = height
	if width == 0 {
		return ""
	}

	// Layout: header(2) + divider(1) + viewport(fills) + divider(1) + input(3+2borders) + status(1)
	inputH := c.input.Height() + 2 // textarea height + border top/bottom
	statusH := 1
	headerH := 2
	dividerH := 2
	vpH := height - headerH - dividerH - inputH - statusH
	if vpH < 3 {
		vpH = 3
	}

	// Header.
	title := c.styles.Header.Render(" Commander Chat ")
	hint := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Render("enter:send  ctrl+l:clear  esc:back")
	hintPadded := lipgloss.NewStyle().Width(width - lipgloss.Width(title) - 2).Align(lipgloss.Right).Render(hint)
	header := lipgloss.JoinHorizontal(lipgloss.Top, title, hintPadded)

	msgCount := fmt.Sprintf(" %d messages ", len(c.messages))
	headerLine2 := lipgloss.NewStyle().Foreground(theme.ColorTextSecondary).Render(msgCount)

	// Viewport.
	c.viewport.Width = width - 2
	c.viewport.Height = vpH

	divTop := lipgloss.NewStyle().
		Foreground(theme.ColorBorderSoft).
		Render(strings.Repeat("─", width))

	vpView := c.viewport.View()

	divBot := lipgloss.NewStyle().
		Foreground(theme.ColorBorderSoft).
		Render(strings.Repeat("─", width))

	// Input area.
	c.input.SetWidth(width - 4)
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorPrimary).
		Padding(0, 1).
		Width(width - 2).
		Render(c.input.View())

	// Status line.
	var statusLine string
	if c.thinking {
		spinner := chatSpinnerFrames[c.spinnerIdx]
		statusLine = lipgloss.NewStyle().
			Foreground(theme.ColorPrimary).
			Render(fmt.Sprintf(" %s Commander is thinking...", spinner))
	} else if c.err != nil {
		statusLine = lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true).
			Render(fmt.Sprintf(" Error: %s", c.err.Error()))
	} else if len(c.messages) == 0 {
		statusLine = lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Render(" Ask Commander anything about your project, past tasks, or architecture.")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		headerLine2,
		divTop,
		vpView,
		divBot,
		inputBox,
		statusLine,
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resize updates internal dimensions for all sub-components.
func (c *CommanderChatScreen) resize() {
	if c.width == 0 {
		return
	}
	inputH := 3
	statusH := 1
	headerH := 2
	dividerH := 2
	vpH := c.height - headerH - dividerH - inputH - statusH - 2 // 2 for input border
	if vpH < 3 {
		vpH = 3
	}
	c.viewport.Width = c.width - 2
	c.viewport.Height = vpH
	c.input.SetWidth(c.width - 4)
	c.refreshViewport()
}

// refreshViewport re-renders all chat messages and sets viewport content.
func (c *CommanderChatScreen) refreshViewport() {
	if c.width == 0 {
		return
	}
	c.viewport.SetContent(c.renderMessages())
}

// renderMessages renders the full conversation history as styled text.
func (c *CommanderChatScreen) renderMessages() string {
	if len(c.messages) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(1).
			Render("No messages yet. Type a question below and press Enter.")
	}

	msgW := c.viewport.Width - 2
	if msgW < 20 {
		msgW = 20
	}

	userStyle := lipgloss.NewStyle().
		Foreground(theme.ColorPrimary).
		Bold(true)

	commanderStyle := lipgloss.NewStyle().
		Foreground(theme.ColorSuccess).
		Bold(true)

	bodyStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		Width(msgW)

	timeStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Italic(true)

	var lines []string
	for _, m := range c.messages {
		switch m.Role {
		case "user":
			label := userStyle.Render("You")
			ts := timeStyle.Render(time.Now().Format("15:04")) // approximate
			header := fmt.Sprintf("%s  %s", label, ts)
			body := bodyStyle.Render(m.Content)
			lines = append(lines, header, body, "")
		case "commander":
			label := commanderStyle.Render("Commander")
			header := label
			body := bodyStyle.Render(m.Content)
			lines = append(lines, header, body, "")
		}
	}

	return strings.Join(lines, "\n")
}
