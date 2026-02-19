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

// Action constants for diff review.
const (
	diffActionMerge = iota // commit + merge into base + clean up (default)
	diffActionCommit
	diffActionKeep
	diffActionRevert
	diffActionDelete
	diffActionCount // sentinel for modular arithmetic
)

// DiffReviewScreen shows git status and diff for a completed execution's worktree.
type DiffReviewScreen struct {
	app    *app.App
	styles theme.Styles

	execution *db.Execution

	status   string // git status output
	diff     string // git diff output
	viewport viewport.Model

	// Action buttons
	actionCursor  int // 0=commit, 1=keep, 2=revert, 3=delete
	confirming    bool
	confirmAction int

	// Post-action message
	resultMessage string

	loaded        bool
	err           error
	width, height int
}

// NewDiffReviewScreen creates a new DiffReviewScreen.
func NewDiffReviewScreen(a *app.App, styles theme.Styles) DiffReviewScreen {
	vp := viewport.New(0, 0)
	return DiffReviewScreen{
		app:      a,
		styles:   styles,
		viewport: vp,
	}
}

// Init satisfies tea.Model.
func (s DiffReviewScreen) Init() tea.Cmd {
	return nil
}

// SetExecution configures the screen for a specific execution and returns the
// command to load git status and diff.
func (s *DiffReviewScreen) SetExecution(exec *db.Execution) tea.Cmd {
	s.execution = exec
	s.loaded = false
	s.err = nil
	s.actionCursor = 0
	s.confirming = false
	s.resultMessage = ""
	return s.loadDiff()
}

// Update processes messages for the diff review screen.
func (s DiffReviewScreen) Update(msg tea.Msg) (DiffReviewScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		s.viewport.Width = msg.Width - 4
		s.viewport.Height = msg.Height - 12
		if s.loaded {
			s.viewport.SetContent(s.renderDiffContent())
		}
		return s, nil

	case DiffLoadedMsg:
		s.loaded = true
		s.status = msg.Status
		s.diff = msg.Diff
		s.viewport.SetContent(s.renderDiffContent())
		return s, nil

	case diffActionDoneMsg:
		s.resultMessage = msg.Message
		s.confirming = false
		return s, nil

	case ErrorMsg:
		s.err = msg.Err
		s.confirming = false
		return s, nil

	case tea.KeyMsg:
		return s.handleKey(msg)

	case tea.MouseMsg:
		return s.handleMouse(msg)
	}

	// Forward to viewport for scrolling.
	var cmd tea.Cmd
	s.viewport, cmd = s.viewport.Update(msg)
	return s, cmd
}

func (s DiffReviewScreen) handleMouse(msg tea.MouseMsg) (DiffReviewScreen, tea.Cmd) {
	// Scroll wheel: forward to viewport.
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		s.viewport, cmd = s.viewport.Update(msg)
		return s, cmd
	}

	// Only handle left-button press for clicks.
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return s, nil
	}

	// If showing result message, click acts like key press to go back.
	if s.resultMessage != "" {
		return s, func() tea.Msg { return NavigateBackMsg{} }
	}

	// Action button bar click: buttons are near the bottom of the view.
	// The buttons are " Commit ", " Keep ", " Revert ", " Delete " laid out horizontally.
	// Approximate: the button bar is rendered a few lines from the bottom.
	// We check if Y is in the lower portion of the screen where buttons render.
	if s.loaded && !s.confirming {
		// Button bar is approximately at height - 6 to height - 4 (within the panel).
		// Use a generous range for the click target.
		buttonY := s.height - 6
		if msg.Y >= buttonY-1 && msg.Y <= buttonY+1 {
			actionLabels := []string{"Commit", "Keep", "Revert", "Delete"}
			x := msg.X - 1 // account for panel padding
			cursor := 0
			for i, label := range actionLabels {
				// Each button renders as " <label> " so width = len(label) + 2.
				btnWidth := len(label) + 2
				if x >= cursor && x < cursor+btnWidth {
					s.actionCursor = i
					return s, nil
				}
				cursor += btnWidth
			}
		}
	}

	return s, nil
}

// Internal message for completed actions.
type diffActionDoneMsg struct{ Message string }

func (s DiffReviewScreen) handleKey(msg tea.KeyMsg) (DiffReviewScreen, tea.Cmd) {
	key := msg.String()

	// If showing result message, any key goes back.
	if s.resultMessage != "" {
		return s, func() tea.Msg { return NavigateBackMsg{} }
	}

	// Confirmation mode.
	if s.confirming {
		switch key {
		case "enter", "y":
			return s, s.executeAction(s.confirmAction)
		case "esc", "n":
			s.confirming = false
			return s, nil
		}
		return s, nil
	}

	switch key {
	case "esc":
		return s, func() tea.Msg { return NavigateBackMsg{} }

	case "left", "h":
		if s.actionCursor > 0 {
			s.actionCursor--
		}
		return s, nil

	case "right", "l":
		if s.actionCursor < diffActionCount-1 {
			s.actionCursor++
		}
		return s, nil

	case "enter":
		// Destructive actions require confirmation.
		if s.actionCursor == diffActionRevert || s.actionCursor == diffActionDelete {
			s.confirming = true
			s.confirmAction = s.actionCursor
			return s, nil
		}
		// Merge also requires confirmation (it modifies the base branch).
		if s.actionCursor == diffActionMerge {
			s.confirming = true
			s.confirmAction = s.actionCursor
			return s, nil
		}
		// Non-destructive actions execute immediately.
		return s, s.executeAction(s.actionCursor)
	}

	// Forward to viewport for scrolling.
	var cmd tea.Cmd
	s.viewport, cmd = s.viewport.Update(msg)
	return s, cmd
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (s *DiffReviewScreen) loadDiff() tea.Cmd {
	a := s.app
	exec := s.execution
	return func() tea.Msg {
		ctx := context.Background()

		status, err := a.Repo().Status(ctx, exec.WorktreePath)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("git status: %w", err)}
		}

		diff, err := a.Repo().DiffAll(ctx, exec.WorktreePath)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("git diff: %w", err)}
		}

		return DiffLoadedMsg{Status: status, Diff: diff}
	}
}

func (s *DiffReviewScreen) executeAction(action int) tea.Cmd {
	a := s.app
	exec := s.execution
	switch action {
	case diffActionMerge:
		return s.mergeChanges(a, exec)
	case diffActionCommit:
		return s.commitChanges(a, exec)
	case diffActionKeep:
		return s.keepChanges()
	case diffActionRevert:
		return s.revertChanges(a, exec)
	case diffActionDelete:
		return s.deleteWorktree(a, exec)
	}
	return nil
}

func (s *DiffReviewScreen) mergeChanges(a *app.App, exec *db.Execution) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Stage and commit in the worktree.
		if err := a.Repo().AddAll(ctx, exec.WorktreePath); err != nil {
			return ErrorMsg{Err: fmt.Errorf("git add: %w", err)}
		}
		commitMsg := fmt.Sprintf("bore-tui: execution #%d", exec.ID)
		if err := a.Repo().Commit(ctx, exec.WorktreePath, commitMsg); err != nil {
			return ErrorMsg{Err: fmt.Errorf("git commit: %w", err)}
		}

		// Merge the exec branch into the base branch.
		baseBranch := exec.BaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
		if err := a.Repo().MergeInto(ctx, baseBranch, exec.ExecBranch); err != nil {
			return ErrorMsg{Err: fmt.Errorf("git merge: %w", err)}
		}

		// Clean up: remove worktree and delete the exec branch.
		if err := a.Repo().RemoveWorktree(ctx, exec.WorktreePath); err != nil {
			return ErrorMsg{Err: fmt.Errorf("remove worktree: %w", err)}
		}
		_ = a.Repo().DeleteBranch(ctx, exec.ExecBranch) // best-effort
		_ = a.Repo().PruneWorktrees(ctx)

		// Mark execution and task as completed.
		_ = a.DB().UpdateExecutionStatus(ctx, exec.ID, db.StatusCompleted)
		_ = a.DB().UpdateTaskStatus(ctx, exec.TaskID, db.StatusCompleted)

		return diffActionDoneMsg{Message: fmt.Sprintf(
			"Merged %s into %s.\nWorktree and branch cleaned up.",
			exec.ExecBranch, baseBranch,
		)}
	}
}

func (s *DiffReviewScreen) commitChanges(a *app.App, exec *db.Execution) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Stage all changes.
		if err := a.Repo().AddAll(ctx, exec.WorktreePath); err != nil {
			return ErrorMsg{Err: fmt.Errorf("git add: %w", err)}
		}

		// Commit with a descriptive message.
		commitMsg := fmt.Sprintf("bore-tui: execution #%d on branch %s", exec.ID, exec.ExecBranch)
		if err := a.Repo().Commit(ctx, exec.WorktreePath, commitMsg); err != nil {
			return ErrorMsg{Err: fmt.Errorf("git commit: %w", err)}
		}

		// Mark execution and task as completed.
		_ = a.DB().UpdateExecutionStatus(ctx, exec.ID, db.StatusCompleted)
		_ = a.DB().UpdateTaskStatus(ctx, exec.TaskID, db.StatusCompleted)

		message := fmt.Sprintf(
			"Changes committed to branch: %s\n\nTo merge: git merge %s",
			exec.ExecBranch, exec.ExecBranch,
		)
		return diffActionDoneMsg{Message: message}
	}
}

func (s *DiffReviewScreen) keepChanges() tea.Cmd {
	return func() tea.Msg {
		return NavigateBackMsg{}
	}
}

func (s *DiffReviewScreen) revertChanges(a *app.App, exec *db.Execution) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		if err := a.Repo().Revert(ctx, exec.WorktreePath, true); err != nil {
			return ErrorMsg{Err: fmt.Errorf("git revert: %w", err)}
		}

		// Mark execution and task as interrupted (reverted).
		_ = a.DB().UpdateExecutionStatus(ctx, exec.ID, db.StatusInterrupted)
		_ = a.DB().UpdateTaskStatus(ctx, exec.TaskID, db.StatusInterrupted)

		return diffActionDoneMsg{Message: "All changes have been reverted."}
	}
}

func (s *DiffReviewScreen) deleteWorktree(a *app.App, exec *db.Execution) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Remove worktree.
		if err := a.Repo().RemoveWorktree(ctx, exec.WorktreePath); err != nil {
			return ErrorMsg{Err: fmt.Errorf("remove worktree: %w", err)}
		}

		// Delete branch.
		if err := a.Repo().DeleteBranch(ctx, exec.ExecBranch); err != nil {
			// Non-fatal: worktree is already removed.
			_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelWarn, "branch_delete_error",
				fmt.Sprintf("Failed to delete branch %s: %v", exec.ExecBranch, err))
		}

		// Prune stale worktrees.
		_ = a.Repo().PruneWorktrees(ctx)

		// Mark execution and task as interrupted (deleted).
		_ = a.DB().UpdateExecutionStatus(ctx, exec.ID, db.StatusInterrupted)
		_ = a.DB().UpdateTaskStatus(ctx, exec.TaskID, db.StatusInterrupted)

		return diffActionDoneMsg{Message: fmt.Sprintf("Worktree and branch %s have been deleted.", exec.ExecBranch)}
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the diff review screen.
func (s DiffReviewScreen) View() string {
	if s.width == 0 {
		return ""
	}

	var sections []string

	// Header.
	if s.execution != nil {
		header := s.styles.Header.Render(
			fmt.Sprintf(" Diff Review: %s ", s.execution.ExecBranch),
		)
		sections = append(sections, header)
	}

	// Error display.
	if s.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
		sections = append(sections, errStyle.Render(fmt.Sprintf("Error: %v", s.err)))
	}

	// Result message (post-action).
	if s.resultMessage != "" {
		msgStyle := lipgloss.NewStyle().Foreground(theme.ColorSuccess).Bold(true)
		sections = append(sections, msgStyle.Render(s.resultMessage))
		sections = append(sections, "")
		sections = append(sections, s.styles.StatusBar.Render("Press any key to go back"))
		return s.styles.Panel.Width(s.width - 2).Render(strings.Join(sections, "\n\n"))
	}

	// Loading state.
	if !s.loaded {
		sections = append(sections, lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).Italic(true).
			Render("Loading diff..."))
		return s.styles.Panel.Width(s.width - 2).Render(strings.Join(sections, "\n\n"))
	}

	// Git status summary.
	if s.status != "" {
		statusLabel := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorPrimary).Render("Git Status:")
		sections = append(sections, statusLabel+"\n"+s.status)
	} else {
		sections = append(sections, lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).Italic(true).
			Render("No changes detected in worktree."))
	}

	// Diff viewport.
	sections = append(sections, s.viewport.View())

	// Confirmation prompt.
	if s.confirming {
		sections = append(sections, s.renderConfirmation())
	} else {
		// Action buttons.
		sections = append(sections, s.renderActionButtons())
	}

	// Footer.
	sections = append(sections, s.renderFooter())

	return s.styles.Panel.Width(s.width - 2).Render(strings.Join(sections, "\n\n"))
}

func (s DiffReviewScreen) renderDiffContent() string {
	if s.diff == "" {
		return lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).Italic(true).
			Render("No diff output. The worktree may have no changes.")
	}

	var lines []string
	for _, line := range strings.Split(s.diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			lines = append(lines, s.styles.DiffAddition.Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			lines = append(lines, s.styles.DiffDeletion.Render(line))
		case strings.HasPrefix(line, "@@"):
			lines = append(lines, lipgloss.NewStyle().
				Foreground(theme.ColorPrimary).Bold(true).Render(line))
		case strings.HasPrefix(line, "diff "):
			lines = append(lines, lipgloss.NewStyle().
				Foreground(theme.ColorTextPrimary).Bold(true).Render(line))
		default:
			lines = append(lines, s.styles.DiffContext.Render(line))
		}
	}

	return strings.Join(lines, "\n")
}

func (s DiffReviewScreen) renderActionButtons() string {
	type actionDef struct{ label string }
	actions := []actionDef{
		{"Merge"},   // diffActionMerge
		{"Commit"},  // diffActionCommit
		{"Keep"},    // diffActionKeep
		{"Revert"},  // diffActionRevert
		{"Delete"},  // diffActionDelete
	}

	var buttons []string
	for i, action := range actions {
		label := fmt.Sprintf(" %s ", action.label)
		switch {
		case i == s.actionCursor:
			buttons = append(buttons, s.styles.ButtonFocused.Render(label))
		case i == diffActionRevert || i == diffActionDelete:
			buttons = append(buttons, s.styles.ButtonDanger.Render(label))
		default:
			buttons = append(buttons, s.styles.Button.Render(label))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, buttons...)
}

func (s DiffReviewScreen) renderConfirmation() string {
	actionNames := []string{
		"merge into base branch and clean up",
		"commit to exec branch",
		"keep changes",
		"revert all changes",
		"delete worktree and branch",
	}
	name := "perform action"
	if s.confirmAction >= 0 && s.confirmAction < len(actionNames) {
		name = actionNames[s.confirmAction]
	}

	warningStyle := lipgloss.NewStyle().
		Foreground(theme.ColorAccent).
		Bold(true)

	prompt := warningStyle.Render(fmt.Sprintf("Are you sure you want to %s?", name))
	hint := s.styles.StatusBar.Render("Enter/y to confirm | Esc/n to cancel")

	return prompt + "\n" + hint
}

func (s DiffReviewScreen) renderFooter() string {
	if s.confirming {
		return ""
	}
	return s.styles.StatusBar.Render("h/l or arrows: select action | Enter: execute | Esc: back")
}
