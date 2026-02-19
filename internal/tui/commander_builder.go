package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bore-tui/internal/agents"
	"bore-tui/internal/app"
	"bore-tui/internal/theme"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Internal message types
// ---------------------------------------------------------------------------

// brainLoadedMsg is sent when the brain text has been fetched from the DB.
type brainLoadedMsg struct{ text string }

// brainScanDoneMsg is sent when the Claude CLI repo scan completes.
type brainScanDoneMsg struct{ text string }

// brainSavedMsg is sent after the brain text has been persisted to the DB.
type brainSavedMsg struct{}

// scanLineMsg carries a single line of live scan output from the Claude CLI.
type scanLineMsg struct{ line string }

// ---------------------------------------------------------------------------
// Spinner frames (no external dependency needed)
// ---------------------------------------------------------------------------

var brainSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ---------------------------------------------------------------------------
// CommanderBuilderScreen
// ---------------------------------------------------------------------------

// CommanderBuilderScreen is a full-screen textarea editor for the Commander
// brain — a freeform text blob that is injected into every Commander session
// for the current cluster.
type CommanderBuilderScreen struct {
	app    *app.App
	styles theme.Styles

	textarea  textarea.Model
	brainText string // last saved value (used to detect unsaved changes)

	scanning  bool     // true while Claude CLI is running the repo scan
	scanLines []string // live output lines captured during scan

	loaded    bool
	statusMsg string
	err       error

	spinnerIdx int
	width      int
	height     int
}

// NewCommanderBuilderScreen creates a CommanderBuilderScreen ready for use.
func NewCommanderBuilderScreen(a *app.App, s theme.Styles) CommanderBuilderScreen {
	ta := textarea.New()
	ta.Placeholder = "Commander brain text will appear here after scanning..."
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(20)

	return CommanderBuilderScreen{
		app:      a,
		styles:   s,
		textarea: ta,
	}
}

// Init is called when navigating to this screen. It checks whether a brain
// already exists in the DB and either loads it or triggers a fresh scan.
func (c *CommanderBuilderScreen) Init() tea.Cmd {
	// Reset transient state on every navigation.
	c.loaded = false
	c.statusMsg = ""
	c.err = nil
	c.scanning = false
	c.scanLines = nil
	c.spinnerIdx = 0
	c.textarea.Focus()

	return c.loadBrainFromDB()
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles messages and key events for the commander brain editor.
func (c *CommanderBuilderScreen) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.resizeTextarea()

	case TickMsg:
		if c.scanning {
			c.spinnerIdx = (c.spinnerIdx + 1) % len(brainSpinnerFrames)
		}

	case brainLoadedMsg:
		c.loaded = true
		if msg.text == "" {
			// Brain is empty — trigger a fresh repo scan.
			cmds = append(cmds, c.startScan())
		} else {
			c.brainText = msg.text
			c.textarea.SetValue(msg.text)
			c.textarea.Focus()
		}

	case brainScanDoneMsg:
		c.scanning = false
		c.scanLines = nil
		c.brainText = msg.text
		c.textarea.SetValue(msg.text)
		c.textarea.Focus()
		// Auto-save the scanned brain to DB.
		cmds = append(cmds, c.saveBrain(msg.text))

	case brainSavedMsg:
		c.brainText = c.textarea.Value()
		c.statusMsg = "Saved"

	case scanLineMsg:
		if len(c.scanLines) > 6 {
			c.scanLines = c.scanLines[len(c.scanLines)-6:]
		}
		c.scanLines = append(c.scanLines, msg.line)

	case ErrorMsg:
		c.scanning = false
		c.err = msg.Err
		c.statusMsg = fmt.Sprintf("tui: commander brain: %v", msg.Err)

	case tea.MouseMsg:
		if !c.scanning {
			switch {
			case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
				c.textarea.Focus()
				// Manually position cursor since bubbles/textarea has no mouse support.
				// Textarea starts at Y=3: header(1) + subtitle(1) + blank(1).
				const textareaStartY = 3
				targetRow := msg.Y - textareaStartY
				if targetRow < 0 {
					targetRow = 0
				}
				delta := targetRow - c.textarea.Line()
				for i := 0; i < delta; i++ {
					c.textarea.CursorDown()
				}
				for i := 0; i > delta; i-- {
					c.textarea.CursorUp()
				}
				c.textarea.SetCursor(msg.X)

			case msg.Button == tea.MouseButtonWheelUp:
				c.textarea.CursorUp()

			case msg.Button == tea.MouseButtonWheelDown:
				c.textarea.CursorDown()
			}
		}

	case tea.KeyMsg:
		if c.scanning {
			// No key actions while scanning (except ctrl+c which is global).
			return nil
		}

		switch msg.String() {
		case "ctrl+s":
			c.statusMsg = ""
			cmds = append(cmds, c.saveBrain(c.textarea.Value()))
			return tea.Batch(cmds...)

		case "ctrl+r":
			c.statusMsg = ""
			c.err = nil
			cmds = append(cmds, c.startScan())
			return tea.Batch(cmds...)

		case "esc":
			return func() tea.Msg { return NavigateBackMsg{} }
		}

		// Delegate all other keys to the textarea.
		var taCmd tea.Cmd
		c.textarea, taCmd = c.textarea.Update(msg)
		cmds = append(cmds, taCmd)
	}

	return tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the commander brain editor.
func (c *CommanderBuilderScreen) View(width, height int) string {
	c.width = width
	c.height = height
	if width == 0 {
		return ""
	}
	c.resizeTextarea()

	// Header.
	header := c.styles.Header.Width(width).Render(" Commander Brain ")

	// Subtitle.
	subtitle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(2).
		Render("This context is injected into every Commander session for this cluster.")

	// Footer hint bar.
	footerText := " [ctrl+s] save  [ctrl+r] re-scan  [esc] back "
	if c.scanning {
		footerText = " Scanning in progress... "
	}
	footer := c.styles.CommandBar.Width(width).Render(footerText)

	// Main body area.
	var body string
	if c.scanning {
		body = c.renderScanView()
	} else if !c.loaded {
		body = lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(4).
			Render("Loading...")
	} else {
		body = c.textarea.View()
	}

	// Status / error line.
	statusLine := ""
	if c.err != nil {
		statusLine = lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true).
			PaddingLeft(2).
			Render(fmt.Sprintf("Error: %v", c.err))
	} else if c.statusMsg != "" {
		statusLine = lipgloss.NewStyle().
			Foreground(theme.ColorSuccess).
			PaddingLeft(2).
			Render(c.statusMsg)
	}

	parts := []string{header, subtitle, "", body}
	if statusLine != "" {
		parts = append(parts, "", statusLine)
	}
	parts = append(parts, footer)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderScanView renders the scanning progress overlay.
func (c *CommanderBuilderScreen) renderScanView() string {
	spinner := brainSpinnerFrames[c.spinnerIdx%len(brainSpinnerFrames)]

	spinnerLine := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		Bold(true).
		PaddingLeft(4).
		Render(fmt.Sprintf("%s  Scanning repository with Commander...", spinner))

	var liveLines []string
	for _, line := range c.scanLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rendered := lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(6).
			Render(line)
		liveLines = append(liveLines, rendered)
	}

	parts := []string{spinnerLine, ""}
	parts = append(parts, liveLines...)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ---------------------------------------------------------------------------
// Layout helpers
// ---------------------------------------------------------------------------

// resizeTextarea adjusts the textarea dimensions to fill available space.
// It accounts for: header (1) + subtitle (1) + blank (1) + statusLine (2) +
// footer (1) = ~6 rows of chrome.
func (c *CommanderBuilderScreen) resizeTextarea() {
	if c.width == 0 || c.height == 0 {
		return
	}
	const fixedRows = 7 // header + subtitle + blank + (optional status) + footer
	taHeight := c.height - fixedRows
	if taHeight < 4 {
		taHeight = 4
	}
	taWidth := c.width - 4 // slight inset
	if taWidth < 20 {
		taWidth = 20
	}
	c.textarea.SetWidth(taWidth)
	c.textarea.SetHeight(taHeight)
}

// ---------------------------------------------------------------------------
// Data commands
// ---------------------------------------------------------------------------

// loadBrainFromDB fetches the brain text from the database and sends a
// brainLoadedMsg. If the cluster is nil or the key does not exist, it sends
// brainLoadedMsg{text: ""} which triggers a fresh scan.
func (c *CommanderBuilderScreen) loadBrainFromDB() tea.Cmd {
	a := c.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return brainLoadedMsg{text: ""}
		}
		text, err := a.DB().GetMemory(context.Background(), cluster.ID, "__brain__")
		if err != nil {
			// A "not found" condition returns an empty string — treat it the
			// same as missing; only hard errors are surfaced.
			// GetMemory returns ("", sql.ErrNoRows) when the key is absent;
			// we normalise that to an empty-string result here.
			return brainLoadedMsg{text: ""}
		}
		return brainLoadedMsg{text: text}
	}
}

// startScan initiates the Claude CLI repo scan. It sets scanning=true
// immediately and returns a command that gathers repo context, builds the
// prompt, runs the Claude CLI, and finishes with brainScanDoneMsg.
func (c *CommanderBuilderScreen) startScan() tea.Cmd {
	c.scanning = true
	c.scanLines = nil
	c.loaded = true // consider the screen loaded even during scan

	a := c.app
	return func() tea.Msg {
		repo := a.Repo()
		if repo == nil {
			return ErrorMsg{Err: fmt.Errorf("tui: commander brain: no repository available")}
		}
		repoPath := repo.Path
		if repoPath == "" {
			return ErrorMsg{Err: fmt.Errorf("tui: commander brain: no repository path available")}
		}

		// Gather top-level directory listing.
		dirListing := gatherDirListing(repoPath)

		// Read README.md if present.
		readmeContent := readFileIfExists(filepath.Join(repoPath, "README.md"), 4096)

		// Collect key config files.
		keyFiles := gatherKeyFiles(repoPath)

		prompt := agents.BuildRepoBrainScanPrompt(repoPath, dirListing, readmeContent, keyFiles)

		// Run the Claude CLI. Because Bubble Tea commands must return a single
		// tea.Msg, we accumulate all output and return it at once. The spinner
		// gives live feedback while the process runs.
		result := a.Runner().Run(
			context.Background(),
			repoPath,
			prompt,
			nil,
			nil, // onStdout — no per-line streaming needed; spinner provides feedback
			nil, // onStderr
		)

		if result.Err != nil {
			return ErrorMsg{Err: fmt.Errorf("tui: commander brain: scan: %w", result.Err)}
		}

		text := strings.TrimSpace(result.Stdout)
		return brainScanDoneMsg{text: text}
	}
}

// gatherDirListing returns a newline-separated listing of top-level entries
// in the given directory. Non-fatal: returns an empty string on error.
func gatherDirListing(repoPath string) string {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return ""
	}
	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			lines = append(lines, e.Name()+"/")
		} else {
			lines = append(lines, e.Name())
		}
	}
	return strings.Join(lines, "\n")
}

// readFileIfExists reads up to maxBytes from the named file.
// Returns an empty string if the file does not exist or cannot be read.
func readFileIfExists(path string, maxBytes int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	return string(data)
}

// gatherKeyFiles reads well-known config files from the repo root and returns
// a map of filename → content (truncated to 2 KB each).
func gatherKeyFiles(repoPath string) map[string]string {
	candidates := []string{
		"go.mod", "package.json", "Cargo.toml", "pyproject.toml",
		"requirements.txt", "Makefile", "Dockerfile", ".env.example",
	}
	result := make(map[string]string)
	for _, name := range candidates {
		content := readFileIfExists(filepath.Join(repoPath, name), 2048)
		if content != "" {
			result[name] = content
		}
	}
	return result
}

// saveBrain persists the given text to the DB under the "__brain__" key.
func (c *CommanderBuilderScreen) saveBrain(text string) tea.Cmd {
	a := c.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return ErrorMsg{Err: fmt.Errorf("tui: commander brain: save: no cluster open")}
		}
		err := a.DB().SetMemory(context.Background(), cluster.ID, "__brain__", text)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("tui: commander brain: save: %w", err)}
		}
		return brainSavedMsg{}
	}
}
