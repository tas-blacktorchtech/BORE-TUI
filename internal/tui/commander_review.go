package tui

import (
	"context"
	"fmt"
	"strings"

	"bore-tui/internal/agents"
	"bore-tui/internal/app"
	"bore-tui/internal/db"
	"bore-tui/internal/git"
	"bore-tui/internal/theme"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommanderReviewScreen implements the multi-step commander review flow.
// Steps: 0=clarifications, 1=options, 2=branch_select, 3=brief, 4=approve.
type CommanderReviewScreen struct {
	app    *app.App
	styles theme.Styles

	task *db.Task

	// Step tracking
	step int // 0=clarifications, 1=options, 2=branch_select, 3=brief, 4=approve

	// Clarifications
	clarifications agents.ClarificationsResponse
	answers        map[string]string
	answerInputs   []textinput.Model
	clarFocus      int

	// Options
	options        agents.OptionsResponse
	selectedOption int

	// Branch selection
	branches     []string
	branchCursor int

	// Brief
	brief agents.ExecutionBrief

	// State
	loading       bool
	err           error
	viewport      viewport.Model
	width, height int
}

// NewCommanderReviewScreen creates a new CommanderReviewScreen.
func NewCommanderReviewScreen(a *app.App, styles theme.Styles) CommanderReviewScreen {
	vp := viewport.New(0, 0)
	return CommanderReviewScreen{
		app:      a,
		styles:   styles,
		answers:  make(map[string]string),
		viewport: vp,
	}
}

// Init satisfies tea.Model. When navigated to, the caller sets the task via
// Update(NavigateMsg) which fires the initial command.
func (s CommanderReviewScreen) Init() tea.Cmd {
	return nil
}

// SetTask configures the screen for a specific task and returns the command
// to start the clarification phase.
func (s *CommanderReviewScreen) SetTask(task *db.Task) tea.Cmd {
	s.task = task
	s.step = 0
	s.loading = true
	s.err = nil
	s.answers = make(map[string]string)
	s.answerInputs = nil
	s.clarFocus = 0
	s.selectedOption = 0
	s.branchCursor = 0
	return s.fetchClarifications()
}

// Update processes messages for the commander review screen.
func (s CommanderReviewScreen) Update(msg tea.Msg) (CommanderReviewScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		s.viewport.Width = msg.Width - 4
		s.viewport.Height = msg.Height - 10
		return s, nil

	case ClarificationsReceivedMsg:
		s.loading = false
		s.clarifications = msg.Response
		if len(msg.Response.Questions) == 0 {
			// No clarifications needed, skip to options.
			s.step = 1
			s.loading = true
			return s, s.fetchOptions()
		}
		// Build text inputs for each question.
		s.answerInputs = make([]textinput.Model, len(msg.Response.Questions))
		for i, q := range msg.Response.Questions {
			ti := textinput.New()
			ti.Placeholder = "Your answer..."
			ti.CharLimit = 500
			ti.Width = s.width - 10
			if ti.Width < 40 {
				ti.Width = 40
			}
			ti.Prompt = fmt.Sprintf("  %s: ", q.ID)
			if i == 0 {
				ti.Focus()
			}
			s.answerInputs[i] = ti
		}
		s.clarFocus = 0
		return s, nil

	case OptionsReceivedMsg:
		s.loading = false
		s.options = msg.Response
		s.selectedOption = 0
		s.step = 1
		return s, nil

	case BranchesLoadedMsg:
		s.loading = false
		s.branches = msg.Branches
		s.branchCursor = 0
		// Default to "main" if present.
		for i, b := range s.branches {
			if b == "main" {
				s.branchCursor = i
				break
			}
		}
		return s, nil

	case BriefReceivedMsg:
		s.loading = false
		s.brief = msg.Response
		s.step = 3
		return s, nil

	case ErrorMsg:
		s.loading = false
		s.err = msg.Err
		return s, nil

	case tea.KeyMsg:
		return s.handleKey(msg)
	}

	// Forward to viewport if scrolling.
	var cmd tea.Cmd
	s.viewport, cmd = s.viewport.Update(msg)
	return s, cmd
}

func (s CommanderReviewScreen) handleKey(msg tea.KeyMsg) (CommanderReviewScreen, tea.Cmd) {
	key := msg.String()

	// Global keys.
	switch key {
	case "esc":
		return s, func() tea.Msg { return NavigateBackMsg{} }
	}

	if s.loading {
		return s, nil
	}

	switch s.step {
	case 0:
		return s.handleClarificationsKey(msg)
	case 1:
		return s.handleOptionsKey(msg)
	case 2:
		return s.handleBranchSelectKey(msg)
	case 3:
		return s.handleBriefKey(msg)
	case 4:
		return s.handleApproveKey(msg)
	}

	return s, nil
}

func (s CommanderReviewScreen) handleClarificationsKey(msg tea.KeyMsg) (CommanderReviewScreen, tea.Cmd) {
	key := msg.String()

	switch key {
	case "tab", "down":
		if len(s.answerInputs) > 0 {
			s.answerInputs[s.clarFocus].Blur()
			s.clarFocus = (s.clarFocus + 1) % len(s.answerInputs)
			s.answerInputs[s.clarFocus].Focus()
		}
		return s, nil

	case "shift+tab", "up":
		if len(s.answerInputs) > 0 {
			s.answerInputs[s.clarFocus].Blur()
			s.clarFocus--
			if s.clarFocus < 0 {
				s.clarFocus = len(s.answerInputs) - 1
			}
			s.answerInputs[s.clarFocus].Focus()
		}
		return s, nil

	case "enter":
		// Collect answers and move to options.
		for i, q := range s.clarifications.Questions {
			if i < len(s.answerInputs) {
				s.answers[q.ID] = s.answerInputs[i].Value()
			}
		}
		s.step = 1
		s.loading = true
		return s, s.fetchOptions()
	}

	// Forward to the focused text input.
	if s.clarFocus >= 0 && s.clarFocus < len(s.answerInputs) {
		var cmd tea.Cmd
		s.answerInputs[s.clarFocus], cmd = s.answerInputs[s.clarFocus].Update(msg)
		return s, cmd
	}

	return s, nil
}

func (s CommanderReviewScreen) handleOptionsKey(msg tea.KeyMsg) (CommanderReviewScreen, tea.Cmd) {
	key := msg.String()

	switch key {
	case "up", "k":
		if s.selectedOption > 0 {
			s.selectedOption--
		}
		return s, nil

	case "down", "j":
		if s.selectedOption < len(s.options.Options)-1 {
			s.selectedOption++
		}
		return s, nil

	case "enter":
		// Move to branch selection.
		s.step = 2
		s.loading = true
		return s, s.fetchBranches()
	}

	return s, nil
}

func (s CommanderReviewScreen) handleBranchSelectKey(msg tea.KeyMsg) (CommanderReviewScreen, tea.Cmd) {
	key := msg.String()

	switch key {
	case "up", "k":
		if s.branchCursor > 0 {
			s.branchCursor--
		}
		return s, nil

	case "down", "j":
		if s.branchCursor < len(s.branches)-1 {
			s.branchCursor++
		}
		return s, nil

	case "enter":
		if len(s.branches) > 0 {
			// Move to brief generation.
			s.step = 3
			s.loading = true
			selectedOptionID := ""
			if s.selectedOption < len(s.options.Options) {
				selectedOptionID = s.options.Options[s.selectedOption].ID
			}
			baseBranch := s.branches[s.branchCursor]
			return s, s.fetchBrief(selectedOptionID, baseBranch)
		}
		return s, nil
	}

	return s, nil
}

func (s CommanderReviewScreen) handleBriefKey(msg tea.KeyMsg) (CommanderReviewScreen, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		s.step = 4
		return s, nil
	}

	// Allow viewport scrolling on the brief.
	var cmd tea.Cmd
	s.viewport, cmd = s.viewport.Update(msg)
	return s, cmd
}

func (s CommanderReviewScreen) handleApproveKey(msg tea.KeyMsg) (CommanderReviewScreen, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter", "y":
		// Start execution: create execution in DB, create worktree+branch, navigate.
		s.loading = true
		return s, s.startExecution()

	case "n":
		return s, func() tea.Msg { return NavigateBackMsg{} }
	}

	return s, nil
}

// execStartData carries the execution, brief, and task to the execution view screen.
type execStartData struct {
	Execution *db.Execution
	Brief     agents.ExecutionBrief
	Task      *db.Task
}

// ---------------------------------------------------------------------------
// Commands (tea.Cmd)
// ---------------------------------------------------------------------------

func (s *CommanderReviewScreen) fetchClarifications() tea.Cmd {
	a := s.app
	task := s.task
	return func() tea.Msg {
		ctx := context.Background()

		cluster := a.Cluster()
		if cluster == nil {
			return ErrorMsg{Err: fmt.Errorf("no cluster open")}
		}

		// Build commander context from DB.
		cmdCtx, err := buildCommanderContext(ctx, a)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("build commander context: %w", err)}
		}

		systemPrompt := agents.BuildCommanderSystemPrompt(cmdCtx)
		userPrompt := agents.BuildClarificationPrompt(task.Prompt)
		fullPrompt := systemPrompt + "\n\n" + userPrompt

		result := a.Runner().Run(ctx, cluster.RepoPath, fullPrompt, nil, nil, nil)
		if result.Err != nil {
			return ErrorMsg{Err: fmt.Errorf("commander clarifications: %w", result.Err)}
		}
		if result.JSONBlock == "" {
			return ErrorMsg{Err: fmt.Errorf("no JSON response from commander (clarifications)")}
		}

		parsed, err := agents.ParseResponse(result.JSONBlock)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("parse clarifications: %w", err)}
		}

		resp, ok := parsed.(agents.ClarificationsResponse)
		if !ok {
			return ErrorMsg{Err: fmt.Errorf("unexpected response type for clarifications: %T", parsed)}
		}

		return ClarificationsReceivedMsg{Response: resp}
	}
}

func (s *CommanderReviewScreen) fetchOptions() tea.Cmd {
	a := s.app
	task := s.task
	answers := s.answers
	return func() tea.Msg {
		ctx := context.Background()

		cluster := a.Cluster()
		if cluster == nil {
			return ErrorMsg{Err: fmt.Errorf("no cluster open")}
		}

		cmdCtx, err := buildCommanderContext(ctx, a)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("build commander context: %w", err)}
		}

		systemPrompt := agents.BuildCommanderSystemPrompt(cmdCtx)
		userPrompt := agents.BuildOptionsPrompt(task.Prompt, answers)
		fullPrompt := systemPrompt + "\n\n" + userPrompt

		result := a.Runner().Run(ctx, cluster.RepoPath, fullPrompt, nil, nil, nil)
		if result.Err != nil {
			return ErrorMsg{Err: fmt.Errorf("commander options: %w", result.Err)}
		}
		if result.JSONBlock == "" {
			return ErrorMsg{Err: fmt.Errorf("no JSON response from commander (options)")}
		}

		parsed, err := agents.ParseResponse(result.JSONBlock)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("parse options: %w", err)}
		}

		resp, ok := parsed.(agents.OptionsResponse)
		if !ok {
			return ErrorMsg{Err: fmt.Errorf("unexpected response type for options: %T", parsed)}
		}

		return OptionsReceivedMsg{Response: resp}
	}
}

func (s *CommanderReviewScreen) fetchBranches() tea.Cmd {
	a := s.app
	return func() tea.Msg {
		ctx := context.Background()
		branches, err := a.Repo().ListBranches(ctx)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("list branches: %w", err)}
		}
		return BranchesLoadedMsg{Branches: branches}
	}
}

func (s *CommanderReviewScreen) fetchBrief(selectedOptionID, baseBranch string) tea.Cmd {
	a := s.app
	task := s.task
	return func() tea.Msg {
		ctx := context.Background()

		cluster := a.Cluster()
		if cluster == nil {
			return ErrorMsg{Err: fmt.Errorf("no cluster open")}
		}

		cmdCtx, err := buildCommanderContext(ctx, a)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("build commander context: %w", err)}
		}

		systemPrompt := agents.BuildCommanderSystemPrompt(cmdCtx)
		userPrompt := agents.BuildExecutionBriefPrompt(task.Prompt, selectedOptionID, baseBranch)
		fullPrompt := systemPrompt + "\n\n" + userPrompt

		result := a.Runner().Run(ctx, cluster.RepoPath, fullPrompt, nil, nil, nil)
		if result.Err != nil {
			return ErrorMsg{Err: fmt.Errorf("commander brief: %w", result.Err)}
		}
		if result.JSONBlock == "" {
			return ErrorMsg{Err: fmt.Errorf("no JSON response from commander (brief)")}
		}

		parsed, err := agents.ParseResponse(result.JSONBlock)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("parse brief: %w", err)}
		}

		resp, ok := parsed.(agents.ExecutionBrief)
		if !ok {
			return ErrorMsg{Err: fmt.Errorf("unexpected response type for brief: %T", parsed)}
		}

		return BriefReceivedMsg{Response: resp}
	}
}

func (s *CommanderReviewScreen) startExecution() tea.Cmd {
	a := s.app
	task := s.task
	brief := s.brief
	return func() tea.Msg {
		ctx := context.Background()

		// Determine the base branch and generate the execution branch name.
		baseBranch := brief.BaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}

		// Look up thread name for branch naming.
		thread, err := a.DB().GetThread(ctx, task.ThreadID)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("get thread for branch name: %w", err)}
		}

		execBranch := git.MakeExecBranch(thread.Name, task.ID, task.Title)
		worktreePath := fmt.Sprintf("%s/worktrees/%s", a.BoreDir(), git.Slugify(task.Title))

		// Create execution record in DB.
		exec, err := a.DB().CreateExecution(ctx, task.ID, task.ClusterID, nil, baseBranch, execBranch, worktreePath)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("create execution: %w", err)}
		}

		// Create git worktree with the new branch.
		if err := a.Repo().CreateWorktreeNewBranch(ctx, worktreePath, execBranch, baseBranch); err != nil {
			return ErrorMsg{Err: fmt.Errorf("create worktree: %w", err)}
		}

		// Update task status to running.
		if err := a.DB().UpdateTaskStatus(ctx, task.ID, db.StatusRunning); err != nil {
			return ErrorMsg{Err: fmt.Errorf("update task status: %w", err)}
		}

		return NavigateMsg{
			Screen: ScreenExecutionView,
			Data: execStartData{
				Execution: exec,
				Brief:     brief,
				Task:      task,
			},
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildCommanderContext gathers all the data needed for the Commander prompt.
func buildCommanderContext(ctx context.Context, a *app.App) (agents.CommanderContext, error) {
	cluster := a.Cluster()
	if cluster == nil {
		return agents.CommanderContext{}, fmt.Errorf("no cluster open")
	}
	clusterID := cluster.ID

	brain, err := a.DB().GetAllMemory(ctx, clusterID)
	if err != nil {
		return agents.CommanderContext{}, fmt.Errorf("get memory: %w", err)
	}

	crews, err := a.DB().ListCrews(ctx, clusterID)
	if err != nil {
		return agents.CommanderContext{}, fmt.Errorf("list crews: %w", err)
	}

	threads, err := a.DB().ListThreads(ctx, clusterID)
	if err != nil {
		return agents.CommanderContext{}, fmt.Errorf("list threads: %w", err)
	}

	lessons, err := a.DB().ListAllLessons(ctx, clusterID)
	if err != nil {
		return agents.CommanderContext{}, fmt.Errorf("list lessons: %w", err)
	}

	// Gather recent agent runs from the most recent executions in this cluster.
	var pastRuns []db.AgentRun
	executions, err := a.DB().ListExecutions(ctx, clusterID)
	if err == nil {
		// Take runs from up to the 5 most recent executions.
		limit := 5
		if len(executions) < limit {
			limit = len(executions)
		}
		for _, ex := range executions[:limit] {
			runs, err := a.DB().GetAgentRuns(ctx, ex.ID)
			if err == nil {
				pastRuns = append(pastRuns, runs...)
			}
		}
	}

	return agents.CommanderContext{
		Brain:    brain,
		Crews:    crews,
		Threads:  threads,
		PastRuns: pastRuns,
		Lessons:  lessons,
	}, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the commander review screen.
func (s CommanderReviewScreen) View() string {
	if s.width == 0 {
		return ""
	}

	var sections []string

	// Header with step indicator.
	stepNames := []string{"Clarifications", "Options", "Branch Select", "Brief", "Approve"}
	header := s.renderStepHeader(stepNames)
	sections = append(sections, header)

	// Task info.
	if s.task != nil {
		taskInfo := s.styles.Header.Render(fmt.Sprintf(" Task: %s ", s.task.Title))
		sections = append(sections, taskInfo)
	}

	// Error display.
	if s.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
		sections = append(sections, errStyle.Render(fmt.Sprintf("Error: %v", s.err)))
		sections = append(sections, "")
		sections = append(sections, s.styles.Button.Render(" Press esc to go back "))
		return s.styles.Panel.Width(s.width - 2).Render(strings.Join(sections, "\n\n"))
	}

	// Loading state.
	if s.loading {
		sections = append(sections, s.renderLoading())
		return s.styles.Panel.Width(s.width - 2).Render(strings.Join(sections, "\n\n"))
	}

	// Step content.
	switch s.step {
	case 0:
		sections = append(sections, s.renderClarifications())
	case 1:
		sections = append(sections, s.renderOptions())
	case 2:
		sections = append(sections, s.renderBranchSelect())
	case 3:
		sections = append(sections, s.renderBrief())
	case 4:
		sections = append(sections, s.renderApprove())
	}

	return s.styles.Panel.Width(s.width - 2).Render(strings.Join(sections, "\n\n"))
}

func (s CommanderReviewScreen) renderStepHeader(stepNames []string) string {
	var tabs []string
	for i, name := range stepNames {
		label := fmt.Sprintf(" %d. %s ", i+1, name)
		if i == s.step {
			tabs = append(tabs, s.styles.TabActive.Render(label))
		} else if i < s.step {
			tabs = append(tabs, s.styles.BadgeCompleted.Render(label))
		} else {
			tabs = append(tabs, s.styles.TabInactive.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (s CommanderReviewScreen) renderLoading() string {
	stepLabels := []string{
		"Asking Commander for clarifying questions...",
		"Generating execution options...",
		"Loading branches...",
		"Generating execution brief...",
		"Creating execution...",
	}
	label := "Loading..."
	if s.step >= 0 && s.step < len(stepLabels) {
		label = stepLabels[s.step]
	}
	return lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Italic(true).
		Render(label)
}

func (s CommanderReviewScreen) renderClarifications() string {
	if len(s.clarifications.Questions) == 0 {
		return "No clarifications needed."
	}

	var lines []string
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTextPrimary).Render("Commander has questions:")
	lines = append(lines, title)
	lines = append(lines, "")

	for i, q := range s.clarifications.Questions {
		qStyle := lipgloss.NewStyle().Foreground(theme.ColorTextPrimary).Bold(true)
		whyStyle := lipgloss.NewStyle().Foreground(theme.ColorTextSecondary).Italic(true)

		lines = append(lines, qStyle.Render(fmt.Sprintf("%s: %s", q.ID, q.Question)))
		lines = append(lines, whyStyle.Render(fmt.Sprintf("   Why: %s", q.Why)))
		if i < len(s.answerInputs) {
			lines = append(lines, s.answerInputs[i].View())
		}
		lines = append(lines, "")
	}

	lines = append(lines, s.styles.StatusBar.Render("Tab/arrows to move between fields | Enter to submit answers"))

	return strings.Join(lines, "\n")
}

func (s CommanderReviewScreen) renderOptions() string {
	if len(s.options.Options) == 0 {
		return "No options generated."
	}

	var lines []string
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTextPrimary).Render("Select an execution option:")
	lines = append(lines, title)
	lines = append(lines, "")

	for i, opt := range s.options.Options {
		var optLines []string

		titleStr := fmt.Sprintf("[%s] %s", opt.ID, opt.Title)
		optLines = append(optLines, lipgloss.NewStyle().Bold(true).Render(titleStr))
		optLines = append(optLines, opt.Summary)

		if len(opt.ApproachSteps) > 0 {
			optLines = append(optLines, "  Steps:")
			for _, step := range opt.ApproachSteps {
				optLines = append(optLines, fmt.Sprintf("    - %s", step))
			}
		}

		if len(opt.Risks) > 0 {
			optLines = append(optLines, "  Risks:")
			for _, risk := range opt.Risks {
				optLines = append(optLines, fmt.Sprintf("    - %s", risk))
			}
		}

		optContent := strings.Join(optLines, "\n")

		if i == s.selectedOption {
			lines = append(lines, s.styles.ListItemSelected.Render("> "+optContent))
		} else {
			lines = append(lines, s.styles.ListItem.Render("  "+optContent))
		}
		lines = append(lines, "")
	}

	lines = append(lines, s.styles.StatusBar.Render("j/k or arrows to navigate | Enter to select"))

	return strings.Join(lines, "\n")
}

func (s CommanderReviewScreen) renderBranchSelect() string {
	if len(s.branches) == 0 {
		return "No branches found."
	}

	var lines []string
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTextPrimary).Render("Select base branch:")
	lines = append(lines, title)
	lines = append(lines, "")

	for i, branch := range s.branches {
		if i == s.branchCursor {
			lines = append(lines, s.styles.ListItemSelected.Render("> "+branch))
		} else {
			lines = append(lines, s.styles.ListItem.Render("  "+branch))
		}
	}

	lines = append(lines, "")
	lines = append(lines, s.styles.StatusBar.Render("j/k or arrows to navigate | Enter to select"))

	return strings.Join(lines, "\n")
}

func (s CommanderReviewScreen) renderBrief() string {
	var lines []string
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTextPrimary).Render("Execution Brief:")
	lines = append(lines, title)
	lines = append(lines, "")

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorPrimary)
	valueStyle := lipgloss.NewStyle().Foreground(theme.ColorTextPrimary)

	lines = append(lines, labelStyle.Render("Task: ")+valueStyle.Render(s.brief.TaskTitle))
	lines = append(lines, labelStyle.Render("Branch: ")+valueStyle.Render(s.brief.BaseBranch))
	lines = append(lines, labelStyle.Render("Thread: ")+valueStyle.Render(s.brief.Thread))
	lines = append(lines, labelStyle.Render("Crew: ")+valueStyle.Render(s.brief.Crew))
	lines = append(lines, labelStyle.Render("Worker Budget: ")+valueStyle.Render(fmt.Sprintf("%d", s.brief.WorkerBudget)))
	lines = append(lines, "")

	if len(s.brief.Scope) > 0 {
		lines = append(lines, labelStyle.Render("Scope:"))
		for _, item := range s.brief.Scope {
			lines = append(lines, "  - "+item)
		}
		lines = append(lines, "")
	}

	if len(s.brief.NotInScope) > 0 {
		lines = append(lines, labelStyle.Render("Not In Scope:"))
		for _, item := range s.brief.NotInScope {
			lines = append(lines, "  - "+item)
		}
		lines = append(lines, "")
	}

	if len(s.brief.SuccessCriteria) > 0 {
		lines = append(lines, labelStyle.Render("Success Criteria:"))
		for _, item := range s.brief.SuccessCriteria {
			lines = append(lines, "  - "+item)
		}
		lines = append(lines, "")
	}

	if len(s.brief.KeyRisks) > 0 {
		lines = append(lines, labelStyle.Render("Key Risks:"))
		for _, item := range s.brief.KeyRisks {
			lines = append(lines, "  - "+item)
		}
		lines = append(lines, "")
	}

	if len(s.brief.RecommendedValidation) > 0 {
		lines = append(lines, labelStyle.Render("Validation Steps:"))
		for _, item := range s.brief.RecommendedValidation {
			lines = append(lines, "  - "+item)
		}
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	s.viewport.SetContent(content)

	lines = []string{
		s.viewport.View(),
		"",
		s.styles.StatusBar.Render("Enter to approve and continue | Esc to go back"),
	}

	return strings.Join(lines, "\n")
}

func (s CommanderReviewScreen) renderApprove() string {
	var lines []string

	title := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTextPrimary).Render("Ready to Start Execution")
	lines = append(lines, title)
	lines = append(lines, "")

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorPrimary)

	if s.task != nil {
		lines = append(lines, labelStyle.Render("Task: ")+s.task.Title)
	}
	if s.brief.TaskTitle != "" {
		lines = append(lines, labelStyle.Render("Brief: ")+s.brief.TaskTitle)
	}
	lines = append(lines, labelStyle.Render("Base Branch: ")+s.brief.BaseBranch)
	if s.selectedOption < len(s.options.Options) {
		lines = append(lines, labelStyle.Render("Option: ")+s.options.Options[s.selectedOption].Title)
	}

	lines = append(lines, "")
	lines = append(lines, s.styles.ButtonFocused.Render(" Start Execution (Enter/y) "))
	lines = append(lines, s.styles.Button.Render(" Cancel (n/Esc) "))
	lines = append(lines, "")
	lines = append(lines, s.styles.StatusBar.Render("Press Enter or y to start | n or Esc to cancel"))

	return strings.Join(lines, "\n")
}
