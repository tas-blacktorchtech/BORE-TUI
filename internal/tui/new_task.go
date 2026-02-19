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

// taskCreatedMsg is sent after a new task has been persisted to the database.
type taskCreatedMsg struct {
	Task *db.Task
}

// ---------------------------------------------------------------------------
// NewTaskScreen
// ---------------------------------------------------------------------------

const (
	newTaskStepForm    = 0
	newTaskStepConfirm = 1
)

// getComplexityOptions returns complexity options mapped to db constants.
func getComplexityOptions() []string {
	return []string{
		db.ComplexityBasic,
		db.ComplexityMedium,
		db.ComplexityComplex,
	}
}

// getModeOptions returns mode options mapped to db constants.
func getModeOptions() []string {
	return []string{
		db.ModeJustGetItDone,
		db.ModeAlertWithIssues,
	}
}

// getModeLabels returns human-readable labels for mode constants.
func getModeLabels() map[string]string {
	return map[string]string{
		db.ModeJustGetItDone:   "Just Get It Done",
		db.ModeAlertWithIssues: "Alert With Issues",
	}
}

// NewTaskScreen is a form-based screen for creating a new task. It collects
// title, prompt, complexity, mode, and thread selection, then persists the
// task and navigates to the commander review screen.
type NewTaskScreen struct {
	app    *app.App
	styles theme.Styles

	titleInput  textinput.Model
	promptInput textinput.Model

	complexity int // index into complexityOptions
	mode       int // index into modeOptions

	threads   []db.Thread
	threadIdx int // selected thread index

	step  int // newTaskStepForm or newTaskStepConfirm
	focus int // 0=title, 1=prompt, 2=complexity, 3=mode, 4=thread

	width, height int
	loaded        bool
	statusMsg     string
}

// NewNewTaskScreen creates a NewTaskScreen ready for use.
func NewNewTaskScreen(a *app.App, s theme.Styles) NewTaskScreen {
	ti := textinput.New()
	ti.Placeholder = "Task title"
	ti.CharLimit = 128
	ti.Width = 60
	ti.Focus()

	pi := textinput.New()
	pi.Placeholder = "Describe what needs to be done..."
	pi.CharLimit = 2048
	pi.Width = 60

	return NewTaskScreen{
		app:         a,
		styles:      s,
		titleInput:  ti,
		promptInput: pi,
	}
}

// Init resets form state and returns the command to load available threads.
func (n *NewTaskScreen) Init() tea.Cmd {
	n.titleInput.Reset()
	n.promptInput.Reset()
	n.complexity = 0
	n.mode = 0
	n.threadIdx = 0
	n.step = newTaskStepForm
	n.focus = 0
	n.loaded = false
	n.statusMsg = ""
	n.titleInput.Focus()
	return n.loadThreads()
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles messages and key events for the new task form.
func (n *NewTaskScreen) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		n.width = msg.Width
		n.height = msg.Height

	case ThreadsLoadedMsg:
		n.threads = msg.Threads
		n.loaded = true

	case taskCreatedMsg:
		// Navigate to commander review with the newly created task.
		return func() tea.Msg {
			return NavigateMsg{Screen: ScreenCommanderReview, Data: msg.Task}
		}

	case ErrorMsg:
		n.statusMsg = fmt.Sprintf("Error: %v", msg.Err)

	case tea.KeyMsg:
		if n.step == newTaskStepConfirm {
			return n.updateConfirm(msg)
		}
		return n.updateForm(msg)
	}

	return nil
}

// updateForm handles key events in the form step.
func (n *NewTaskScreen) updateForm(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {

	case "tab":
		n.focus = (n.focus + 1) % 5
		n.syncFocus()
		return nil

	case "shift+tab":
		n.focus = (n.focus + 4) % 5
		n.syncFocus()
		return nil

	case "left":
		switch n.focus {
		case 2: // complexity
			if n.complexity > 0 {
				n.complexity--
			}
		case 3: // mode
			if n.mode > 0 {
				n.mode--
			}
		case 4: // thread
			if n.threadIdx > 0 {
				n.threadIdx--
			}
		}
		return nil

	case "right":
		switch n.focus {
		case 2:
			if n.complexity < len(getComplexityOptions())-1 {
				n.complexity++
			}
		case 3:
			if n.mode < len(getModeOptions())-1 {
				n.mode++
			}
		case 4:
			if n.threadIdx < len(n.threads)-1 {
				n.threadIdx++
			}
		}
		return nil

	case "ctrl+s", "ctrl+n":
		if n.titleInput.Value() == "" {
			n.statusMsg = "Title is required"
			return nil
		}
		if n.promptInput.Value() == "" {
			n.statusMsg = "Prompt is required"
			return nil
		}
		if len(n.threads) == 0 {
			n.statusMsg = "No threads available - create one first"
			return nil
		}
		n.step = newTaskStepConfirm
		n.statusMsg = ""
		return nil

	case "esc":
		return func() tea.Msg { return NavigateBackMsg{} }
	}

	// Delegate to focused text input.
	var cmd tea.Cmd
	switch n.focus {
	case 0:
		n.titleInput, cmd = n.titleInput.Update(msg)
	case 1:
		n.promptInput, cmd = n.promptInput.Update(msg)
	}
	return cmd
}

// updateConfirm handles key events in the confirmation step.
func (n *NewTaskScreen) updateConfirm(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter", "y", "Y":
		return n.createTask()
	case "esc", "n", "N":
		n.step = newTaskStepForm
		return nil
	}
	return nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the new task screen.
func (n *NewTaskScreen) View(width, height int) string {
	n.width = width
	n.height = height
	if width == 0 {
		return ""
	}

	header := n.styles.Header.Width(width).Render(" New Task ")

	var body string
	if n.step == newTaskStepConfirm {
		body = n.renderConfirm()
	} else {
		body = n.renderForm()
	}

	statusLine := ""
	if n.statusMsg != "" {
		statusLine = lipgloss.NewStyle().
			Foreground(theme.ColorWarning).
			PaddingLeft(2).
			Render(n.statusMsg)
	}

	var helpText string
	if n.step == newTaskStepConfirm {
		helpText = " [enter/y] create task  [esc/n] go back to form "
	} else {
		helpText = " [tab] next field  [left/right] select option  [ctrl+s] submit  [esc] cancel "
	}
	helpBar := n.styles.CommandBar.Width(width).Render(helpText)

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

func (n *NewTaskScreen) renderForm() string {
	labelStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(4)

	// Title input.
	titleStyle := n.styles.Input
	if n.focus == 0 {
		titleStyle = n.styles.InputFocused
	}
	titleSection := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Title:"),
		"    "+titleStyle.Render(n.titleInput.View()),
	)

	// Prompt input.
	promptStyle := n.styles.Input
	if n.focus == 1 {
		promptStyle = n.styles.InputFocused
	}
	promptSection := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Prompt:"),
		"    "+promptStyle.Render(n.promptInput.View()),
	)

	// Complexity selector.
	complexitySection := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Complexity:"),
		"    "+n.renderSelector(getComplexityOptions(), n.complexity, n.focus == 2),
	)

	// Mode selector.
	modeSection := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Mode:"),
		"    "+n.renderModeSelector(n.focus == 3),
	)

	// Thread selector.
	threadSection := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Thread:"),
		"    "+n.renderThreadSelector(n.focus == 4),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleSection,
		"",
		promptSection,
		"",
		complexitySection,
		"",
		modeSection,
		"",
		threadSection,
	)
}

func (n *NewTaskScreen) renderSelector(options []string, selected int, focused bool) string {
	var parts []string
	for i, opt := range options {
		if i == selected {
			parts = append(parts, n.styles.ButtonFocused.Render(opt))
		} else if focused {
			parts = append(parts, n.styles.Button.Render(opt))
		} else {
			parts = append(parts, n.styles.Button.
				Foreground(theme.ColorTextSecondary).
				Render(opt))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

func (n *NewTaskScreen) renderModeSelector(focused bool) string {
	var parts []string
	for i, opt := range getModeOptions() {
		label := getModeLabels()[opt]
		if label == "" {
			label = opt
		}
		if i == n.mode {
			parts = append(parts, n.styles.ButtonFocused.Render(label))
		} else if focused {
			parts = append(parts, n.styles.Button.Render(label))
		} else {
			parts = append(parts, n.styles.Button.
				Foreground(theme.ColorTextSecondary).
				Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

func (n *NewTaskScreen) renderThreadSelector(focused bool) string {
	if len(n.threads) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Render("(no threads available)")
	}

	var parts []string
	// Show a window of threads around the selected index.
	windowSize := 5
	start := n.threadIdx - windowSize/2
	if start < 0 {
		start = 0
	}
	end := start + windowSize
	if end > len(n.threads) {
		end = len(n.threads)
		start = end - windowSize
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Render("< "))
	}

	for i := start; i < end; i++ {
		label := dashTruncate(n.threads[i].Name, 16)
		if i == n.threadIdx {
			parts = append(parts, n.styles.ButtonFocused.Render(label))
		} else if focused {
			parts = append(parts, n.styles.Button.Render(label))
		} else {
			parts = append(parts, n.styles.Button.
				Foreground(theme.ColorTextSecondary).
				Render(label))
		}
	}

	if end < len(n.threads) {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Render(" >"))
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

func (n *NewTaskScreen) renderConfirm() string {
	labelStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(4)
	valueStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		Bold(true).
		PaddingLeft(6)

	title := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		Bold(true).
		PaddingLeft(4).
		Render("Confirm Task Creation")

	threadName := "(none)"
	if n.threadIdx < len(n.threads) {
		threadName = n.threads[n.threadIdx].Name
	}

	modeLabel := getModeLabels()[getModeOptions()[n.mode]]
	if modeLabel == "" {
		modeLabel = getModeOptions()[n.mode]
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		labelStyle.Render("Title:"),
		valueStyle.Render(n.titleInput.Value()),
		"",
		labelStyle.Render("Prompt:"),
		valueStyle.Render(dashTruncate(n.promptInput.Value(), 80)),
		"",
		labelStyle.Render("Complexity:"),
		valueStyle.Render(getComplexityOptions()[n.complexity]),
		"",
		labelStyle.Render("Mode:"),
		valueStyle.Render(modeLabel),
		"",
		labelStyle.Render("Thread:"),
		valueStyle.Render(threadName),
		"",
		lipgloss.NewStyle().
			Foreground(theme.ColorSuccess).
			PaddingLeft(4).
			Render("Press enter or 'y' to create, esc or 'n' to go back"),
	)
}

// ---------------------------------------------------------------------------
// Focus management
// ---------------------------------------------------------------------------

func (n *NewTaskScreen) syncFocus() {
	n.titleInput.Blur()
	n.promptInput.Blur()

	switch n.focus {
	case 0:
		n.titleInput.Focus()
	case 1:
		n.promptInput.Focus()
	}
	// Focus 2-4 are selector fields; no text input focus needed.
}

// ---------------------------------------------------------------------------
// Data commands
// ---------------------------------------------------------------------------

func (n *NewTaskScreen) loadThreads() tea.Cmd {
	a := n.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return ThreadsLoadedMsg{}
		}
		threads, err := a.DB().ListThreads(context.Background(), cluster.ID)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return ThreadsLoadedMsg{Threads: threads}
	}
}

func (n *NewTaskScreen) createTask() tea.Cmd {
	title := n.titleInput.Value()
	prompt := n.promptInput.Value()
	complexity := getComplexityOptions()[n.complexity]
	mode := getModeOptions()[n.mode]
	threadIdx := n.threadIdx
	a := n.app

	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return ErrorMsg{Err: fmt.Errorf("no cluster open")}
		}

		threads, err := a.DB().ListThreads(context.Background(), cluster.ID)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		if threadIdx >= len(threads) {
			return ErrorMsg{Err: fmt.Errorf("selected thread index out of range")}
		}
		threadID := threads[threadIdx].ID

		task, err := a.DB().CreateTask(context.Background(),
			cluster.ID, threadID, title, prompt, complexity, mode)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return taskCreatedMsg{Task: task}
	}
}
