package tui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"bore-tui/internal/app"
	"bore-tui/internal/config"
	"bore-tui/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// configField represents a single editable configuration value.
type configField struct {
	label string
	key   string
	value string
	kind  string // "string", "int", "bool"
}

// ConfigEditorScreen displays and edits the cluster configuration.
type ConfigEditorScreen struct {
	app     *app.App
	styles  theme.Styles
	fields  []configField
	cursor  int
	editing bool
	input   textinput.Model
	err     error
	saved   bool
}

// NewConfigEditorScreen creates a new config editor screen.
func NewConfigEditorScreen(a *app.App, s theme.Styles) ConfigEditorScreen {
	ti := textinput.New()
	ti.CharLimit = 256
	ti.Width = 40

	return ConfigEditorScreen{
		app:    a,
		styles: s,
		input:  ti,
	}
}

// loadFields populates the editable fields from the current configuration.
// If no config is loaded (no cluster open), it loads from DefaultConfig.
func (s *ConfigEditorScreen) loadFields() {
	cfg := s.app.Config()
	if cfg == nil {
		def := config.DefaultConfig()
		cfg = &def
	}

	s.fields = []configField{
		{label: "UI Theme", key: "ui.theme", value: cfg.UI.Theme, kind: "string"},
		{label: "Claude CLI Path", key: "agents.claude_cli_path", value: cfg.Agents.ClaudeCLIPath, kind: "string"},
		{label: "Default Model", key: "agents.default_model", value: cfg.Agents.DefaultModel, kind: "string"},
		{label: "Commander Context Limit", key: "agents.commander_context_limit", value: strconv.Itoa(cfg.Agents.CommanderContextLimit), kind: "int"},
		{label: "Max Total Workers", key: "agents.max_total_workers", value: strconv.Itoa(cfg.Agents.MaxTotalWorkers), kind: "int"},
		{label: "Max Workers (Basic)", key: "agents.max_workers_basic", value: strconv.Itoa(cfg.Agents.MaxWorkersBasic), kind: "int"},
		{label: "Max Workers (Medium)", key: "agents.max_workers_medium", value: strconv.Itoa(cfg.Agents.MaxWorkersMedium), kind: "int"},
		{label: "Max Workers (Complex)", key: "agents.max_workers_complex", value: strconv.Itoa(cfg.Agents.MaxWorkersComplex), kind: "int"},
		{label: "Worktree Strategy", key: "git.worktree_strategy", value: cfg.Git.WorktreeStrategy, kind: "string"},
		{label: "Review Required", key: "git.review_required", value: strconv.FormatBool(cfg.Git.ReviewRequired), kind: "bool"},
		{label: "Auto Commit", key: "git.auto_commit", value: strconv.FormatBool(cfg.Git.AutoCommit), kind: "bool"},
		{label: "Log Level", key: "logging.level", value: cfg.Logging.Level, kind: "string"},
		{label: "Log To Console", key: "logging.to_console", value: strconv.FormatBool(cfg.Logging.ToConsole), kind: "bool"},
		{label: "Log Rotation (MB)", key: "logging.rotation_mb", value: strconv.Itoa(cfg.Logging.RotationMB), kind: "int"},
	}

	s.cursor = 0
	s.editing = false
	s.err = nil
	s.saved = false
}

// Update handles messages for the config editor screen.
func (s *ConfigEditorScreen) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		km := DefaultKeyMap()

		// If editing a field, handle the text input.
		if s.editing {
			switch msg.String() {
			case "enter":
				return s.commitEdit()
			case "esc":
				s.editing = false
				s.input.Blur()
				return nil
			}
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return cmd
		}

		// Navigation mode.
		switch {
		case msg.String() == "esc":
			return func() tea.Msg { return NavigateBackMsg{} }
		case key.Matches(msg, km.Up):
			if s.cursor > 0 {
				s.cursor--
			}
		case key.Matches(msg, km.Down):
			if s.cursor < len(s.fields)-1 {
				s.cursor++
			}
		case key.Matches(msg, km.Enter):
			return s.startEdit()
		case msg.String() == "s":
			return s.save()
		}
	}
	return nil
}

// startEdit begins editing the selected field.
func (s *ConfigEditorScreen) startEdit() tea.Cmd {
	if len(s.fields) == 0 {
		return nil
	}

	f := &s.fields[s.cursor]

	// Bool fields toggle immediately without a text input.
	if f.kind == "bool" {
		if f.value == "true" {
			f.value = "false"
		} else {
			f.value = "true"
		}
		s.saved = false
		return nil
	}

	s.editing = true
	s.input.SetValue(f.value)
	s.input.Focus()
	s.input.CursorEnd()
	return textinput.Blink
}

// commitEdit applies the edited value back to the field.
func (s *ConfigEditorScreen) commitEdit() tea.Cmd {
	if s.cursor >= len(s.fields) {
		s.editing = false
		return nil
	}

	f := &s.fields[s.cursor]
	newVal := strings.TrimSpace(s.input.Value())

	// Validate int fields.
	if f.kind == "int" {
		if _, err := strconv.Atoi(newVal); err != nil {
			s.err = fmt.Errorf("%s must be an integer", f.label)
			return nil
		}
	}

	f.value = newVal
	s.editing = false
	s.saved = false
	s.err = nil
	s.input.Blur()
	return nil
}

// save writes the edited fields back to a Config and persists it.
func (s *ConfigEditorScreen) save() tea.Cmd {
	cfg := s.buildConfig()

	if err := config.Validate(cfg); err != nil {
		s.err = err
		return nil
	}

	boreDir := s.app.BoreDir()
	if boreDir == "" {
		s.err = fmt.Errorf("no cluster open; cannot save config")
		return nil
	}

	cfgPath := filepath.Join(boreDir, "config.json")
	if err := config.Save(cfg, cfgPath); err != nil {
		s.err = fmt.Errorf("save config: %w", err)
		return nil
	}

	s.saved = true
	s.err = nil
	return func() tea.Msg {
		return StatusMsg("Configuration saved")
	}
}

// buildConfig assembles a Config from the current field values.
func (s *ConfigEditorScreen) buildConfig() *config.Config {
	cfg := config.DefaultConfig()

	for _, f := range s.fields {
		switch f.key {
		case "ui.theme":
			cfg.UI.Theme = f.value
		case "agents.claude_cli_path":
			cfg.Agents.ClaudeCLIPath = f.value
		case "agents.default_model":
			cfg.Agents.DefaultModel = f.value
		case "agents.commander_context_limit":
			cfg.Agents.CommanderContextLimit, _ = strconv.Atoi(f.value)
		case "agents.max_total_workers":
			cfg.Agents.MaxTotalWorkers, _ = strconv.Atoi(f.value)
		case "agents.max_workers_basic":
			cfg.Agents.MaxWorkersBasic, _ = strconv.Atoi(f.value)
		case "agents.max_workers_medium":
			cfg.Agents.MaxWorkersMedium, _ = strconv.Atoi(f.value)
		case "agents.max_workers_complex":
			cfg.Agents.MaxWorkersComplex, _ = strconv.Atoi(f.value)
		case "git.worktree_strategy":
			cfg.Git.WorktreeStrategy = f.value
		case "git.review_required":
			cfg.Git.ReviewRequired = f.value == "true"
		case "git.auto_commit":
			cfg.Git.AutoCommit = f.value == "true"
		case "logging.level":
			cfg.Logging.Level = f.value
		case "logging.to_console":
			cfg.Logging.ToConsole = f.value == "true"
		case "logging.rotation_mb":
			cfg.Logging.RotationMB, _ = strconv.Atoi(f.value)
		}
	}

	return &cfg
}

// View renders the config editor screen.
func (s *ConfigEditorScreen) View(width, height int) string {
	var sections []string

	// Header.
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.ColorTextPrimary).
		Background(theme.ColorPrimary).
		Padding(0, 3).
		Render("Configuration")

	sections = append(sections, title, "")

	// Error / saved status.
	if s.err != nil {
		errLine := lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true).
			Render(fmt.Sprintf("  Error: %s", s.err.Error()))
		sections = append(sections, errLine, "")
	} else if s.saved {
		savedLine := lipgloss.NewStyle().
			Foreground(theme.ColorSuccess).
			Render("  Configuration saved successfully")
		sections = append(sections, savedLine, "")
	}

	if len(s.fields) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Italic(true).
			Render("  No configuration loaded")
		sections = append(sections, empty)
		return s.wrapContent(sections, width, height)
	}

	// Compute max label width for alignment.
	maxLabel := 0
	for _, f := range s.fields {
		if len(f.label) > maxLabel {
			maxLabel = len(f.label)
		}
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Width(maxLabel + 2)

	valueStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary)

	boolTrueStyle := lipgloss.NewStyle().
		Foreground(theme.ColorSuccess).
		Bold(true)

	boolFalseStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary)

	for i, f := range s.fields {
		label := labelStyle.Render(f.label)

		var val string
		if s.editing && i == s.cursor {
			val = s.input.View()
		} else if f.kind == "bool" {
			if f.value == "true" {
				val = boolTrueStyle.Render("true")
			} else {
				val = boolFalseStyle.Render("false")
			}
		} else {
			val = valueStyle.Render(f.value)
		}

		line := fmt.Sprintf("  %s  %s", label, val)

		if i == s.cursor {
			prefix := lipgloss.NewStyle().
				Foreground(theme.ColorPrimary).
				Bold(true).
				Render(">")
			line = fmt.Sprintf(" %s%s  %s", prefix, label, val)
			// Highlight the row.
			line = lipgloss.NewStyle().
				Background(theme.ColorPanel).
				Render(line)
		}

		sections = append(sections, line)
	}

	// Footer hints.
	sections = append(sections, "")
	var hints []string
	if s.editing {
		hints = append(hints, "Enter: confirm", "Esc: cancel")
	} else {
		hints = append(hints, "Enter: edit", "s: save", "Esc: back")
	}
	hintLine := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Italic(true).
		Render("  " + strings.Join(hints, "  |  "))
	sections = append(sections, hintLine)

	return s.wrapContent(sections, width, height)
}

// wrapContent pads and positions the content within the viewport.
func (s *ConfigEditorScreen) wrapContent(sections []string, width, height int) string {
	content := strings.Join(sections, "\n")
	contentHeight := lipgloss.Height(content)

	padTop := 0
	if height > contentHeight {
		padTop = (height - contentHeight) / 4
	}

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		PaddingTop(padTop).
		PaddingLeft(4).
		Render(content)
}
