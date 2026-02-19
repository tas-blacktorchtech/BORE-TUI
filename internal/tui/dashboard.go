package tui

import (
	"context"
	"fmt"
	"strings"

	"bore-tui/internal/app"
	"bore-tui/internal/db"
	"bore-tui/internal/theme"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// DashboardScreen — the main 3-pane cluster dashboard
// ---------------------------------------------------------------------------

// DashboardScreen displays crews/threads, tasks, and detail information in a
// three-column layout. It is the primary screen after opening a cluster.
type DashboardScreen struct {
	app    *app.App
	styles theme.Styles

	// Pane focus: 0=left, 1=center, 2=right
	focusedPane int

	// Left pane — tabbed: 0=crews, 1=threads
	leftTab    int
	crews      []db.Crew
	threads    []db.Thread
	leftCursor int

	// Center pane — task list
	tasks        []db.Task
	executions   []db.Execution
	centerCursor int
	filterThread int64 // 0 = show all

	// Right pane — detail view with sub-tabs
	rightTab     int // 0=summary, 1=logs, 2=diff
	selectedTask *db.Task
	selectedExec *db.Execution
	detailText   string
	viewport     viewport.Model

	width, height int
	loaded        bool
}

// NewDashboardScreen creates a DashboardScreen ready for use.
func NewDashboardScreen(a *app.App, s theme.Styles) DashboardScreen {
	vp := viewport.New(0, 0)
	return DashboardScreen{
		app:      a,
		styles:   s,
		viewport: vp,
	}
}

// Init returns commands to load initial data from the database.
func (d *DashboardScreen) Init() tea.Cmd {
	return tea.Batch(
		d.loadCrews(),
		d.loadThreads(),
		d.loadTasks(),
		d.loadExecutions(),
	)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles messages and key events for the dashboard.
func (d *DashboardScreen) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		d.recalcViewport()

	case CrewsLoadedMsg:
		d.crews = msg.Crews
		d.loaded = true

	case ThreadsLoadedMsg:
		d.threads = msg.Threads

	case TasksLoadedMsg:
		d.tasks = msg.Tasks
		d.clampCenterCursor()

	case ExecutionsLoadedMsg:
		d.executions = msg.Executions

	case tea.KeyMsg:
		switch msg.String() {

		// Pane cycling
		case "tab":
			d.focusedPane = (d.focusedPane + 1) % 3
		case "shift+tab":
			d.focusedPane = (d.focusedPane + 2) % 3

		// Vertical navigation
		case "up", "k":
			d.moveCursorUp()
		case "down", "j":
			d.moveCursorDown()

		// Left pane tab switching (only when left pane focused)
		case "1":
			if d.focusedPane == 0 {
				d.leftTab = 0
				d.leftCursor = 0
			}
		case "2":
			if d.focusedPane == 0 {
				d.leftTab = 1
				d.leftCursor = 0
			}

		// Selection
		case "enter":
			cmd := d.handleEnter()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

		// Shortcuts
		case "n":
			return func() tea.Msg {
				return NavigateMsg{Screen: ScreenNewTask}
			}
		case "c":
			return func() tea.Msg {
				return NavigateMsg{Screen: ScreenCrewManager}
			}
		case "b":
			return func() tea.Msg {
				return NavigateMsg{Screen: ScreenCommanderBuilder}
			}

		// Clear filter or navigate back
		case "esc":
			if d.filterThread != 0 {
				d.filterThread = 0
				d.clampCenterCursor()
			} else {
				return func() tea.Msg { return NavigateBackMsg{} }
			}

		// Refresh data
		case "r":
			cmds = append(cmds,
				d.loadCrews(),
				d.loadThreads(),
				d.loadTasks(),
				d.loadExecutions(),
			)
		}
	}

	return tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the three-pane dashboard layout.
func (d *DashboardScreen) View(width, height int) string {
	d.width = width
	d.height = height
	if width == 0 {
		return ""
	}

	// Calculate pane widths.
	leftW := width / 4
	rightW := width / 4
	centerW := width - leftW - rightW

	// Available height for pane content (subtract bottom bar).
	contentH := height - 3
	if contentH < 4 {
		contentH = 4
	}

	left := d.renderLeftPane(leftW, contentH)
	center := d.renderCenterPane(centerW, contentH)
	right := d.renderRightPane(rightW, contentH)

	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)
	bottom := d.renderBottomBar(width)

	return lipgloss.JoinVertical(lipgloss.Left, panes, bottom)
}

// ---------------------------------------------------------------------------
// Data loading commands
// ---------------------------------------------------------------------------

func (d *DashboardScreen) loadCrews() tea.Cmd {
	a := d.app
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

func (d *DashboardScreen) loadThreads() tea.Cmd {
	a := d.app
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

func (d *DashboardScreen) loadTasks() tea.Cmd {
	a := d.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return TasksLoadedMsg{}
		}
		tasks, err := a.DB().ListTasks(context.Background(), cluster.ID)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return TasksLoadedMsg{Tasks: tasks}
	}
}

func (d *DashboardScreen) loadExecutions() tea.Cmd {
	a := d.app
	return func() tea.Msg {
		cluster := a.Cluster()
		if cluster == nil {
			return ExecutionsLoadedMsg{}
		}
		execs, err := a.DB().ListExecutions(context.Background(), cluster.ID)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return ExecutionsLoadedMsg{Executions: execs}
	}
}

// ---------------------------------------------------------------------------
// Cursor movement helpers
// ---------------------------------------------------------------------------

func (d *DashboardScreen) moveCursorUp() {
	switch d.focusedPane {
	case 0:
		n := d.leftListLen()
		if n == 0 {
			return
		}
		d.leftCursor = (d.leftCursor - 1 + n) % n
	case 1:
		n := len(d.filteredTasks())
		if n == 0 {
			return
		}
		d.centerCursor = (d.centerCursor - 1 + n) % n
		d.syncSelectedTask()
	case 2:
		d.viewport.LineUp(1)
	}
}

func (d *DashboardScreen) moveCursorDown() {
	switch d.focusedPane {
	case 0:
		n := d.leftListLen()
		if n == 0 {
			return
		}
		d.leftCursor = (d.leftCursor + 1) % n
	case 1:
		n := len(d.filteredTasks())
		if n == 0 {
			return
		}
		d.centerCursor = (d.centerCursor + 1) % n
		d.syncSelectedTask()
	case 2:
		d.viewport.LineDown(1)
	}
}

func (d *DashboardScreen) leftListLen() int {
	if d.leftTab == 0 {
		return len(d.crews)
	}
	return len(d.threads)
}

func (d *DashboardScreen) handleEnter() tea.Cmd {
	switch d.focusedPane {
	case 0:
		// Left pane: filter tasks by selected thread.
		if d.leftTab == 1 && d.leftCursor < len(d.threads) {
			t := d.threads[d.leftCursor]
			d.filterThread = t.ID
			d.clampCenterCursor()
		}
	case 1:
		// Center pane: select a task to show detail on right pane.
		filtered := d.filteredTasks()
		if d.centerCursor < len(filtered) {
			task := filtered[d.centerCursor]
			d.selectedTask = &task
			d.selectedExec = nil
			d.updateDetailText()
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Filtering and detail
// ---------------------------------------------------------------------------

func (d *DashboardScreen) clampCenterCursor() {
	n := len(d.filteredTasks())
	if n == 0 {
		d.centerCursor = 0
	} else if d.centerCursor >= n {
		d.centerCursor = n - 1
	}
}

func (d *DashboardScreen) filteredTasks() []db.Task {
	if d.filterThread == 0 {
		return d.tasks
	}
	var out []db.Task
	for _, t := range d.tasks {
		if t.ThreadID == d.filterThread {
			out = append(out, t)
		}
	}
	return out
}

func (d *DashboardScreen) syncSelectedTask() {
	filtered := d.filteredTasks()
	if d.centerCursor < len(filtered) {
		task := filtered[d.centerCursor]
		d.selectedTask = &task
		d.selectedExec = nil
		d.updateDetailText()
	}
}

func (d *DashboardScreen) updateDetailText() {
	if d.selectedTask == nil {
		d.detailText = ""
		d.viewport.SetContent("")
		return
	}

	t := d.selectedTask
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Title: %s\n", t.Title))
	b.WriteString(fmt.Sprintf("Status: %s\n", t.Status))
	b.WriteString(fmt.Sprintf("Complexity: %s\n", t.Complexity))
	b.WriteString(fmt.Sprintf("Mode: %s\n", t.Mode))
	b.WriteString(fmt.Sprintf("Thread: %s\n", d.threadName(t.ThreadID)))
	b.WriteString(fmt.Sprintf("Created: %s\n", t.CreatedAt.Format("2006-01-02 15:04")))
	b.WriteString("\n--- Prompt ---\n")
	b.WriteString(t.Prompt)

	// Show executions for this task.
	taskExecs := d.executionsForTask(t.ID)
	if len(taskExecs) > 0 {
		b.WriteString("\n\n--- Executions ---\n")
		for _, e := range taskExecs {
			b.WriteString(fmt.Sprintf("  #%d  %s  branch: %s\n", e.ID, e.Status, e.ExecBranch))
		}
	}

	d.detailText = b.String()
	d.viewport.SetContent(d.detailText)
}

func (d *DashboardScreen) executionsForTask(taskID int64) []db.Execution {
	var out []db.Execution
	for _, e := range d.executions {
		if e.TaskID == taskID {
			out = append(out, e)
		}
	}
	return out
}

func (d *DashboardScreen) recalcViewport() {
	rightW := d.width / 4
	contentH := d.height - 8
	if contentH < 1 {
		contentH = 1
	}
	innerW := rightW - 6
	if innerW < 10 {
		innerW = 10
	}
	d.viewport.Width = innerW
	d.viewport.Height = contentH
	if d.detailText != "" {
		d.viewport.SetContent(d.detailText)
	}
}

// ---------------------------------------------------------------------------
// Rendering helpers
// ---------------------------------------------------------------------------

func (d *DashboardScreen) renderLeftPane(w, h int) string {
	panelStyle := d.styles.Panel
	if d.focusedPane == 0 {
		panelStyle = d.styles.PanelFocused
	}
	panelStyle = panelStyle.Width(w - 4).Height(h)

	// Tab header
	var crewTab, threadTab string
	if d.leftTab == 0 {
		crewTab = d.styles.TabActive.Render(" Crews ")
		threadTab = d.styles.TabInactive.Render(" Threads ")
	} else {
		crewTab = d.styles.TabInactive.Render(" Crews ")
		threadTab = d.styles.TabActive.Render(" Threads ")
	}
	tabs := lipgloss.JoinHorizontal(lipgloss.Bottom, crewTab, threadTab)

	// List content
	var content string
	if d.leftTab == 0 {
		content = d.renderCrewList(h - 3)
	} else {
		content = d.renderThreadList(h - 3)
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, tabs, "", content)
	return panelStyle.Render(inner)
}

func (d *DashboardScreen) renderCrewList(maxLines int) string {
	if len(d.crews) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(1).
			Render("No crews defined")
	}
	var lines []string
	for i, c := range d.crews {
		if i >= maxLines {
			break
		}
		label := dashTruncate(c.Name, 20)
		if d.focusedPane == 0 && i == d.leftCursor {
			lines = append(lines, d.styles.ListItemSelected.Render(label))
		} else {
			lines = append(lines, d.styles.ListItem.Render(label))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (d *DashboardScreen) renderThreadList(maxLines int) string {
	if len(d.threads) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(1).
			Render("No threads created")
	}
	var lines []string
	for i, t := range d.threads {
		if i >= maxLines {
			break
		}
		label := dashTruncate(t.Name, 20)
		if d.focusedPane == 0 && i == d.leftCursor {
			lines = append(lines, d.styles.ListItemSelected.Render(label))
		} else {
			lines = append(lines, d.styles.ListItem.Render(label))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (d *DashboardScreen) renderCenterPane(w, h int) string {
	panelStyle := d.styles.Panel
	if d.focusedPane == 1 {
		panelStyle = d.styles.PanelFocused
	}
	panelStyle = panelStyle.Width(w - 4).Height(h)

	header := d.styles.Header.Render(" Tasks ")

	// Filter indicator
	filterLine := ""
	if d.filterThread != 0 {
		threadName := d.threadName(d.filterThread)
		filterLine = lipgloss.NewStyle().
			Foreground(theme.ColorWarning).
			Render(fmt.Sprintf("Filtered: %s (esc to clear)", threadName))
	}

	filtered := d.filteredTasks()
	var content string
	if len(filtered) == 0 {
		content = lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(1).
			Render("No tasks found")
	} else {
		maxLines := h - 5
		if maxLines < 1 {
			maxLines = 1
		}
		var lines []string
		for i, t := range filtered {
			if i >= maxLines {
				break
			}
			badge := d.statusBadge(t.Status)
			title := dashTruncate(t.Title, w-20)
			complexity := fmt.Sprintf("[%s]", t.Complexity)

			line := fmt.Sprintf("%s %s %s", badge, title, complexity)
			if d.focusedPane == 1 && i == d.centerCursor {
				lines = append(lines, d.styles.ListItemSelected.Render(line))
			} else {
				lines = append(lines, d.styles.ListItem.Render(line))
			}
		}
		content = lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	parts := []string{header}
	if filterLine != "" {
		parts = append(parts, filterLine)
	}
	parts = append(parts, "", content)
	inner := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return panelStyle.Render(inner)
}

func (d *DashboardScreen) renderRightPane(w, h int) string {
	panelStyle := d.styles.Panel
	if d.focusedPane == 2 {
		panelStyle = d.styles.PanelFocused
	}
	panelStyle = panelStyle.Width(w - 4).Height(h)

	header := d.styles.Header.Render(" Detail ")

	if d.selectedTask == nil {
		placeholder := lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			PaddingLeft(1).
			Render("Select a task to view details")
		inner := lipgloss.JoinVertical(lipgloss.Left, header, "", placeholder)
		return panelStyle.Render(inner)
	}

	// Sub-tabs for detail view
	tabs := d.renderDetailTabs()

	// Recalculate viewport dimensions.
	innerW := w - 8
	if innerW < 10 {
		innerW = 10
	}
	innerH := h - 6
	if innerH < 1 {
		innerH = 1
	}
	d.viewport.Width = innerW
	d.viewport.Height = innerH

	vpContent := d.viewport.View()

	inner := lipgloss.JoinVertical(lipgloss.Left, header, tabs, "", vpContent)
	return panelStyle.Render(inner)
}

func (d *DashboardScreen) renderDetailTabs() string {
	labels := []string{" Summary ", " Logs ", " Diff "}
	var tabs []string
	for i, label := range labels {
		if i == d.rightTab {
			tabs = append(tabs, d.styles.TabActive.Render(label))
		} else {
			tabs = append(tabs, d.styles.TabInactive.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, tabs...)
}

func (d *DashboardScreen) renderBottomBar(totalWidth int) string {
	clusterName := "(no cluster)"
	if d.app.Cluster() != nil {
		clusterName = d.app.Cluster().Name
	}

	execCount := len(d.executions)
	runningCount := 0
	for _, e := range d.executions {
		if e.Status == db.StatusRunning {
			runningCount++
		}
	}

	maxWorkers := 0
	if d.app.Config() != nil {
		maxWorkers = d.app.Config().Agents.MaxTotalWorkers
	}

	info := fmt.Sprintf(" %s | Tasks: %d | Exec: %d | Running: %d/%d ",
		clusterName, len(d.tasks), execCount, runningCount, maxWorkers)

	keys := " tab:pane  n:task  c:crews  b:brain  r:refresh "

	barStyle := d.styles.StatusBar.Width(totalWidth)

	gap := totalWidth - lipgloss.Width(info) - lipgloss.Width(keys)
	if gap < 0 {
		gap = 0
	}
	return barStyle.Render(info + strings.Repeat(" ", gap) + keys)
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

func (d *DashboardScreen) statusBadge(status string) string {
	switch status {
	case db.StatusRunning:
		return d.styles.BadgeRunning.Render("RUN")
	case db.StatusCompleted:
		return d.styles.BadgeCompleted.Render("DONE")
	case db.StatusFailed:
		return d.styles.BadgeFailed.Render("FAIL")
	case db.StatusInterrupted:
		return d.styles.BadgeInterrupted.Render("INT")
	case db.StatusPending:
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Padding(0, 1).
			Render("PEND")
	case db.StatusReview:
		return lipgloss.NewStyle().
			Foreground(theme.ColorWarning).
			Padding(0, 1).
			Render("REV")
	case db.StatusDiffReview:
		return lipgloss.NewStyle().
			Foreground(theme.ColorWarning).
			Padding(0, 1).
			Render("DIFF")
	default:
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Padding(0, 1).
			Render(strings.ToUpper(status))
	}
}

func (d *DashboardScreen) threadName(id int64) string {
	for _, t := range d.threads {
		if t.ID == id {
			return t.Name
		}
	}
	return fmt.Sprintf("#%d", id)
}

// dashTruncate shortens s to maxLen characters, appending an ellipsis if needed.
func dashTruncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
