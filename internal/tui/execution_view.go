package tui

import (
	"context"
	"fmt"
	"strings"

	"bore-tui/internal/agents"
	"bore-tui/internal/app"
	"bore-tui/internal/db"
	"bore-tui/internal/theme"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Execution step constants
// ---------------------------------------------------------------------------

const (
	execStepIdle           = 0
	execStepBossPlan       = 1
	execStepRunningWorkers = 2
	execStepBossSummary    = 3
	execStepDone           = 4
)

// ---------------------------------------------------------------------------
// Internal message types for phased execution
// ---------------------------------------------------------------------------

type taskLoadedMsg struct{ Task *db.Task }
type executionOutputMsg struct{ Line string }
type agentRunsLoadedMsg struct{ Runs []db.AgentRun }

type bossPlanDoneMsg struct {
	plan *agents.BossPlan
	err  error
}

type workerDoneMsg struct {
	result   agents.WorkerResult
	agentRun *db.AgentRun
	err      error
}

type bossSummaryDoneMsg struct {
	summary *agents.BossSummary
	err     error
}

// executionStartedInternalMsg signals that execution has been marked started
// in the DB and the boss plan phase should begin.
type executionStartedInternalMsg struct {
	task *db.Task
	err  error
}

// ---------------------------------------------------------------------------
// ExecutionViewScreen
// ---------------------------------------------------------------------------

// ExecutionViewScreen shows a running or completed execution.
type ExecutionViewScreen struct {
	app    *app.App
	styles theme.Styles

	execution *db.Execution
	task      *db.Task
	brief     *agents.ExecutionBrief
	agentRuns []db.AgentRun

	// Live output
	outputLines []string
	viewport    viewport.Model

	// Tab: 0=overview, 1=live output, 2=workers
	tab int

	// Phased execution state
	execStep      int // execStepIdle..execStepDone
	bossPlan      *agents.BossPlan
	workerResults []agents.WorkerResult
	currentWorker int
	crew          *db.Crew // cached crew for the execution

	// State
	running       bool
	err           error
	width, height int
}

// NewExecutionViewScreen creates a new ExecutionViewScreen.
func NewExecutionViewScreen(a *app.App, styles theme.Styles) ExecutionViewScreen {
	vp := viewport.New(0, 0)
	return ExecutionViewScreen{
		app:      a,
		styles:   styles,
		viewport: vp,
	}
}

// Init satisfies tea.Model.
func (s ExecutionViewScreen) Init() tea.Cmd {
	return nil
}

// SetExecution configures the screen for a specific execution and returns the
// command to start or load it.
func (s *ExecutionViewScreen) SetExecution(exec *db.Execution) tea.Cmd {
	s.execution = exec
	s.task = nil
	s.brief = nil
	s.tab = 0
	s.err = nil
	s.outputLines = nil
	s.agentRuns = nil
	s.execStep = execStepIdle
	s.bossPlan = nil
	s.workerResults = nil
	s.currentWorker = 0
	s.crew = nil

	// Load the associated task.
	return tea.Batch(
		s.loadTask(),
		s.maybeStartExecution(),
	)
}

// SetExecutionWithBrief configures the screen with an execution, brief, and
// task so the brief is available for the boss plan phase.
func (s *ExecutionViewScreen) SetExecutionWithBrief(exec *db.Execution, brief agents.ExecutionBrief, task *db.Task) tea.Cmd {
	s.execution = exec
	s.task = task
	s.brief = &brief
	s.tab = 0
	s.err = nil
	s.outputLines = nil
	s.agentRuns = nil
	s.execStep = execStepIdle
	s.bossPlan = nil
	s.workerResults = nil
	s.currentWorker = 0
	s.crew = nil

	return s.maybeStartExecution()
}

// Update processes messages for the execution view screen.
func (s ExecutionViewScreen) Update(msg tea.Msg) (ExecutionViewScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		s.viewport.Width = msg.Width - 4
		s.viewport.Height = msg.Height - 10
		s.updateViewportContent()
		return s, nil

	case taskLoadedMsg:
		s.task = msg.Task
		s.updateViewportContent()
		return s, nil

	case executionStartedInternalMsg:
		if msg.err != nil {
			s.running = false
			s.err = msg.err
			s.updateViewportContent()
			return s, nil
		}
		// Store the task if it was loaded in the start phase.
		if msg.task != nil && s.task == nil {
			s.task = msg.task
		}
		s.execStep = execStepBossPlan
		s.outputLines = append(s.outputLines, "Execution started. Running Boss plan...")
		s.updateViewportContent()
		return s, s.runBossPlan()

	case executionOutputMsg:
		s.outputLines = append(s.outputLines, msg.Line)
		if s.tab == 1 {
			s.updateViewportContent()
			s.viewport.GotoBottom()
		}
		return s, nil

	case bossPlanDoneMsg:
		if msg.err != nil {
			s.running = false
			s.err = msg.err
			s.updateViewportContent()
			return s, nil
		}
		s.bossPlan = msg.plan
		s.execStep = execStepRunningWorkers
		s.currentWorker = 0
		s.workerResults = nil
		s.outputLines = append(s.outputLines,
			fmt.Sprintf("Boss plan complete: %d steps, %d workers needed.",
				len(msg.plan.Steps), len(msg.plan.NeedsWorkers)))
		s.updateViewportContent()

		// Start the first worker, or skip to summary if no workers needed.
		if len(s.bossPlan.NeedsWorkers) == 0 {
			s.execStep = execStepBossSummary
			s.outputLines = append(s.outputLines, "No workers needed. Running Boss summary...")
			s.updateViewportContent()
			return s, s.runBossSummary()
		}
		s.outputLines = append(s.outputLines,
			fmt.Sprintf("Starting worker 1/%d: %s...",
				len(s.bossPlan.NeedsWorkers), s.bossPlan.NeedsWorkers[0].Role))
		s.updateViewportContent()
		return s, s.runNextWorker(s.bossPlan.NeedsWorkers[0])

	case workerDoneMsg:
		if msg.err != nil {
			s.outputLines = append(s.outputLines,
				fmt.Sprintf("Worker error: %v", msg.err))
		} else {
			s.workerResults = append(s.workerResults, msg.result)
			s.outputLines = append(s.outputLines,
				fmt.Sprintf("Worker %q finished: %s", msg.result.Summary, msg.result.Outcome))
		}

		// Reload agent runs to show in the workers tab.
		reloadCmd := s.loadAgentRuns()

		s.currentWorker++
		if s.currentWorker < len(s.bossPlan.NeedsWorkers) {
			// Start the next worker.
			need := s.bossPlan.NeedsWorkers[s.currentWorker]
			s.outputLines = append(s.outputLines,
				fmt.Sprintf("Starting worker %d/%d: %s...",
					s.currentWorker+1, len(s.bossPlan.NeedsWorkers), need.Role))
			s.updateViewportContent()
			return s, tea.Batch(reloadCmd, s.runNextWorker(need))
		}

		// All workers done; start boss summary.
		s.execStep = execStepBossSummary
		s.outputLines = append(s.outputLines, "All workers complete. Running Boss summary...")
		s.updateViewportContent()
		return s, tea.Batch(reloadCmd, s.runBossSummary())

	case bossSummaryDoneMsg:
		s.execStep = execStepDone
		s.running = false
		if msg.err != nil {
			s.outputLines = append(s.outputLines,
				fmt.Sprintf("Boss summary error: %v", msg.err))
		} else if msg.summary != nil {
			s.outputLines = append(s.outputLines,
				fmt.Sprintf("Execution finished: %s", msg.summary.Outcome))
		} else {
			s.outputLines = append(s.outputLines, "Execution finished.")
		}
		s.updateViewportContent()
		// Reload agent runs and update execution status from DB.
		return s, s.loadAgentRuns()

	case ExecutionDoneMsg:
		s.running = false
		s.execution.Status = msg.Status
		s.updateViewportContent()
		// Reload agent runs.
		return s, s.loadAgentRuns()

	case agentRunsLoadedMsg:
		s.agentRuns = msg.Runs
		s.updateViewportContent()
		return s, nil

	case ErrorMsg:
		s.running = false
		s.err = msg.Err
		s.updateViewportContent()
		return s, nil

	case tea.KeyMsg:
		return s.handleKey(msg)

	case tea.MouseMsg:
		return s.handleMouse(msg)
	}

	// Forward to viewport.
	var cmd tea.Cmd
	s.viewport, cmd = s.viewport.Update(msg)
	return s, cmd
}

func (s ExecutionViewScreen) handleMouse(msg tea.MouseMsg) (ExecutionViewScreen, tea.Cmd) {
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

	// Tab bar click: tabs are at Y=0 (first rendered line inside the panel).
	// Tab labels are " Overview ", " Output ", " Workers " (each padded with spaces).
	// The panel has ~1 char left padding, so account for that.
	if msg.Y <= 1 {
		tabs := []string{"Overview", "Output", "Workers"}
		x := msg.X - 1 // account for panel padding
		cursor := 0
		for i, tab := range tabs {
			// Each tab renders as " <name> " so width = len(name) + 2.
			tabWidth := len(tab) + 2
			if x >= cursor && x < cursor+tabWidth {
				s.tab = i
				s.updateViewportContent()
				return s, nil
			}
			cursor += tabWidth
		}
	}

	return s, nil
}

func (s ExecutionViewScreen) handleKey(msg tea.KeyMsg) (ExecutionViewScreen, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		return s, func() tea.Msg { return NavigateBackMsg{} }

	case "tab":
		s.tab = (s.tab + 1) % 3
		s.updateViewportContent()
		return s, nil

	case "d":
		// Navigate to diff review if execution is done.
		if !s.running && s.execution != nil &&
			(s.execution.Status == db.StatusCompleted ||
				s.execution.Status == db.StatusFailed ||
				s.execution.Status == db.StatusDiffReview) {
			return s, func() tea.Msg {
				return NavigateMsg{
					Screen: ScreenDiffReview,
					Data:   s.execution,
				}
			}
		}
		return s, nil

	case "r":
		// Refresh agent runs.
		return s, s.loadAgentRuns()
	}

	// Forward to viewport for scrolling.
	var cmd tea.Cmd
	s.viewport, cmd = s.viewport.Update(msg)
	return s, cmd
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (s *ExecutionViewScreen) loadTask() tea.Cmd {
	a := s.app
	exec := s.execution
	return func() tea.Msg {
		ctx := context.Background()
		task, err := a.DB().GetTask(ctx, exec.TaskID)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("load task: %w", err)}
		}
		return taskLoadedMsg{Task: task}
	}
}

func (s *ExecutionViewScreen) loadAgentRuns() tea.Cmd {
	a := s.app
	exec := s.execution
	return func() tea.Msg {
		ctx := context.Background()
		runs, err := a.DB().GetAgentRuns(ctx, exec.ID)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("load agent runs: %w", err)}
		}
		return agentRunsLoadedMsg{Runs: runs}
	}
}

func (s *ExecutionViewScreen) maybeStartExecution() tea.Cmd {
	exec := s.execution
	if exec == nil {
		return nil
	}

	// Only start if execution is still pending.
	if exec.Status != db.StatusPending {
		// Already running or finished; just load agent runs.
		if exec.Status == db.StatusRunning {
			s.running = true
		}
		return s.loadAgentRuns()
	}

	s.running = true
	return s.startExecution()
}

// startExecution marks the execution as started in the DB and returns a
// message that triggers the boss plan phase via Update.
func (s *ExecutionViewScreen) startExecution() tea.Cmd {
	a := s.app
	exec := s.execution
	task := s.task

	return func() tea.Msg {
		ctx := context.Background()

		// Mark execution as started.
		if err := a.DB().SetExecutionStarted(ctx, exec.ID); err != nil {
			return executionStartedInternalMsg{err: fmt.Errorf("set execution started: %w", err)}
		}

		_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelInfo, "execution_start", "Execution started")

		// If task was not passed in (e.g. navigated from dashboard), load it.
		localTask := task
		if localTask == nil {
			var err error
			localTask, err = a.DB().GetTask(ctx, exec.TaskID)
			if err != nil {
				markFailed(ctx, a, exec.ID, nil)
				return executionStartedInternalMsg{err: fmt.Errorf("load task for execution: %w", err)}
			}
		}

		return executionStartedInternalMsg{task: localTask}
	}
}

// runBossPlan runs JUST the Boss plan phase and returns a bossPlanDoneMsg.
func (s *ExecutionViewScreen) runBossPlan() tea.Cmd {
	a := s.app
	exec := s.execution
	task := s.task
	brief := s.brief
	return func() tea.Msg {
		ctx := context.Background()

		// Load task if not available yet.
		localTask := task
		if localTask == nil {
			var err error
			localTask, err = a.DB().GetTask(ctx, exec.TaskID)
			if err != nil {
				markFailed(ctx, a, exec.ID, nil)
				return bossPlanDoneMsg{err: fmt.Errorf("load task: %w", err)}
			}
		}

		// Load crew if assigned.
		var crew *db.Crew
		if exec.CrewID != nil {
			var err error
			crew, err = a.DB().GetCrew(ctx, *exec.CrewID)
			if err != nil {
				markFailed(ctx, a, exec.ID, localTask)
				return bossPlanDoneMsg{err: fmt.Errorf("load crew: %w", err)}
			}
		}

		_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelInfo, "boss_plan", "Running Boss plan phase")

		// Build the brief.
		var useBrief agents.ExecutionBrief
		if brief != nil {
			useBrief = *brief
		} else {
			useBrief = buildBriefFromExec(exec, localTask)
		}

		bossCtx := agents.BossContext{
			Crew:         crew,
			Brief:        useBrief,
			TaskPrompt:   localTask.Prompt,
			Mode:         localTask.Mode,
			WorkerBudget: 3, // Default budget for V1.
		}

		bossSystemPrompt := agents.BuildBossSystemPrompt(bossCtx)
		bossPlanPrompt := agents.BuildBossPlanPrompt(bossCtx)
		fullBossPrompt := bossSystemPrompt + "\n\n" + bossPlanPrompt

		bossResult := a.Runner().Run(ctx, exec.WorktreePath, fullBossPrompt, nil, nil, nil)
		if bossResult.Err != nil {
			markFailed(ctx, a, exec.ID, localTask)
			return bossPlanDoneMsg{err: fmt.Errorf("boss plan: %w", bossResult.Err)}
		}

		if bossResult.JSONBlock == "" {
			markFailed(ctx, a, exec.ID, localTask)
			return bossPlanDoneMsg{err: fmt.Errorf("boss plan: no JSON response")}
		}

		parsed, err := agents.ParseResponse(bossResult.JSONBlock)
		if err != nil {
			markFailed(ctx, a, exec.ID, localTask)
			return bossPlanDoneMsg{err: fmt.Errorf("boss plan parse: %w", err)}
		}

		plan, ok := parsed.(agents.BossPlan)
		if !ok {
			markFailed(ctx, a, exec.ID, localTask)
			return bossPlanDoneMsg{err: fmt.Errorf("boss plan: unexpected type %T", parsed)}
		}

		// Save boss plan as an agent run.
		_, _ = a.DB().CreateAgentRun(ctx, exec.ID, db.AgentTypeBoss, "planner",
			fullBossPrompt, fmt.Sprintf("Plan with %d steps, %d workers", len(plan.Steps), len(plan.NeedsWorkers)),
			db.OutcomeSuccess, strings.Join(plan.EstimatedFiles, ", "))

		_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelInfo, "boss_plan_done",
			fmt.Sprintf("Boss plan: %d steps, %d workers needed", len(plan.Steps), len(plan.NeedsWorkers)))

		return bossPlanDoneMsg{plan: &plan}
	}
}

// runNextWorker runs a single worker and returns a workerDoneMsg.
func (s *ExecutionViewScreen) runNextWorker(workerNeed agents.WorkerNeed) tea.Cmd {
	a := s.app
	exec := s.execution
	crew := s.crew
	workerIdx := s.currentWorker
	totalWorkers := len(s.bossPlan.NeedsWorkers)
	return func() tea.Msg {
		ctx := context.Background()

		_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelInfo, "worker_start",
			fmt.Sprintf("Starting worker %d/%d: %s", workerIdx+1, totalWorkers, workerNeed.Role))

		// Acquire scheduler slot.
		if err := a.Scheduler().Acquire(ctx); err != nil {
			_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelError, "scheduler_error", err.Error())
			return workerDoneMsg{err: fmt.Errorf("scheduler: %w", err)}
		}

		workerCtx := agents.WorkerContext{
			Role:            workerNeed.Role,
			Goal:            workerNeed.Goal,
			FilesOrPaths:    workerNeed.FilesOrPaths,
			AllowedCommands: workerNeed.Commands,
			SuccessCriteria: workerNeed.SuccessCriteria,
		}
		if crew != nil {
			workerCtx.CrewObjective = crew.Objective
			workerCtx.CrewConstraints = crew.Constraints
		}

		workerPrompt := agents.BuildWorkerSystemPrompt(workerCtx)
		workerResult := a.Runner().Run(ctx, exec.WorktreePath, workerPrompt, nil, nil, nil)

		a.Scheduler().Release()

		if workerResult.Err != nil {
			_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelError, "worker_error",
				fmt.Sprintf("Worker %s failed: %v", workerNeed.Role, workerResult.Err))

			_, _ = a.DB().CreateAgentRun(ctx, exec.ID, db.AgentTypeWorker, workerNeed.Role,
				workerPrompt, fmt.Sprintf("Failed: %v", workerResult.Err),
				db.OutcomeFailed, "")
			return workerDoneMsg{err: fmt.Errorf("worker %s: %w", workerNeed.Role, workerResult.Err)}
		}

		if workerResult.JSONBlock == "" {
			_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelWarn, "worker_no_json",
				fmt.Sprintf("Worker %s returned no JSON", workerNeed.Role))

			_, _ = a.DB().CreateAgentRun(ctx, exec.ID, db.AgentTypeWorker, workerNeed.Role,
				workerPrompt, "No JSON output",
				db.OutcomeFailed, "")
			return workerDoneMsg{err: fmt.Errorf("worker %s: no JSON response", workerNeed.Role)}
		}

		parsedWorker, err := agents.ParseResponse(workerResult.JSONBlock)
		if err != nil {
			_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelWarn, "worker_parse_error",
				fmt.Sprintf("Worker %s parse error: %v", workerNeed.Role, err))

			_, _ = a.DB().CreateAgentRun(ctx, exec.ID, db.AgentTypeWorker, workerNeed.Role,
				workerPrompt, fmt.Sprintf("Parse error: %v", err),
				db.OutcomeFailed, "")
			return workerDoneMsg{err: fmt.Errorf("worker %s parse: %w", workerNeed.Role, err)}
		}

		wr, ok := parsedWorker.(agents.WorkerResult)
		if !ok {
			_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelWarn, "worker_type_error",
				fmt.Sprintf("Worker %s returned unexpected type %T", workerNeed.Role, parsedWorker))
			return workerDoneMsg{err: fmt.Errorf("worker %s: unexpected type %T", workerNeed.Role, parsedWorker)}
		}

		// Save worker run to DB.
		agentRun, _ := a.DB().CreateAgentRun(ctx, exec.ID, db.AgentTypeWorker, workerNeed.Role,
			workerPrompt, wr.Summary, wr.Outcome, strings.Join(wr.FilesChanged, ", "))

		_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelInfo, "worker_done",
			fmt.Sprintf("Worker %s finished: %s", workerNeed.Role, wr.Outcome))

		return workerDoneMsg{result: wr, agentRun: agentRun}
	}
}

// runBossSummary runs the Boss summary phase and returns a bossSummaryDoneMsg.
func (s *ExecutionViewScreen) runBossSummary() tea.Cmd {
	a := s.app
	exec := s.execution
	task := s.task
	brief := s.brief
	workerResults := s.workerResults

	return func() tea.Msg {
		ctx := context.Background()

		_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelInfo, "boss_summary", "Running Boss summary phase")

		// Load task if needed.
		localTask := task
		if localTask == nil {
			var err error
			localTask, err = a.DB().GetTask(ctx, exec.TaskID)
			if err != nil {
				return bossSummaryDoneMsg{err: fmt.Errorf("load task: %w", err)}
			}
		}

		// Load crew if assigned.
		var crew *db.Crew
		if exec.CrewID != nil {
			var err error
			crew, err = a.DB().GetCrew(ctx, *exec.CrewID)
			if err != nil {
				return bossSummaryDoneMsg{err: fmt.Errorf("load crew: %w", err)}
			}
		}

		// Build the brief.
		var useBrief agents.ExecutionBrief
		if brief != nil {
			useBrief = *brief
		} else {
			useBrief = buildBriefFromExec(exec, localTask)
		}

		bossCtx := agents.BossContext{
			Crew:         crew,
			Brief:        useBrief,
			TaskPrompt:   localTask.Prompt,
			Mode:         localTask.Mode,
			WorkerBudget: 3,
		}

		bossSystemPrompt := agents.BuildBossSystemPrompt(bossCtx)
		bossSummaryPrompt := bossSystemPrompt + "\n\n" + agents.BuildBossSummaryPrompt(workerResults)
		summaryResult := a.Runner().Run(ctx, exec.WorktreePath, bossSummaryPrompt, nil, nil, nil)

		finalStatus := db.StatusCompleted
		var bossSummary *agents.BossSummary

		if summaryResult.Err != nil {
			_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelError, "boss_summary_error",
				fmt.Sprintf("Boss summary failed: %v", summaryResult.Err))
		} else if summaryResult.JSONBlock != "" {
			parsedSummary, err := agents.ParseResponse(summaryResult.JSONBlock)
			if err == nil {
				if bs, ok := parsedSummary.(agents.BossSummary); ok {
					bossSummary = &bs

					// Save summary as agent run.
					_, _ = a.DB().CreateAgentRun(ctx, exec.ID, db.AgentTypeBoss, "summarizer",
						bossSummaryPrompt, strings.Join(bs.WhatChanged, "; "),
						bs.Outcome, strings.Join(bs.FilesTouched, ", "))

					// Save lessons.
					for _, lesson := range bs.Lessons {
						_ = a.DB().CreateLesson(ctx, exec.ID, db.AgentTypeBoss, lesson.LessonType, lesson.Content)
					}

					// Set final status based on outcome.
					switch bs.Outcome {
					case "failed":
						finalStatus = db.StatusFailed
					case "partial":
						finalStatus = db.StatusDiffReview
					default:
						finalStatus = db.StatusDiffReview
					}
				}
			}
		}

		// Mark execution finished.
		if err := a.DB().SetExecutionFinished(ctx, exec.ID, finalStatus); err != nil {
			return bossSummaryDoneMsg{err: fmt.Errorf("set execution finished: %w", err)}
		}

		// Update task status.
		_ = a.DB().UpdateTaskStatus(ctx, localTask.ID, finalStatus)

		_ = a.DB().CreateEvent(ctx, exec.ID, db.LevelInfo, "execution_done",
			fmt.Sprintf("Execution finished with status: %s", finalStatus))

		return bossSummaryDoneMsg{summary: bossSummary}
	}
}

// markFailed is a helper to mark an execution as failed.
func markFailed(ctx context.Context, a *app.App, execID int64, task *db.Task) {
	_ = a.DB().SetExecutionFinished(ctx, execID, db.StatusFailed)
	if task != nil {
		_ = a.DB().UpdateTaskStatus(ctx, task.ID, db.StatusFailed)
	}
}

// buildBriefFromExec creates a minimal ExecutionBrief from an execution and task.
// In V1, the brief may not have been fully saved; this provides defaults.
func buildBriefFromExec(exec *db.Execution, task *db.Task) agents.ExecutionBrief {
	return agents.ExecutionBrief{
		Type:       "execution_brief",
		BaseBranch: exec.BaseBranch,
		TaskTitle:  task.Title,
	}
}

// updateViewportContent refreshes the viewport content based on the current tab.
func (s *ExecutionViewScreen) updateViewportContent() {
	switch s.tab {
	case 0:
		s.viewport.SetContent(s.overviewContent())
	case 1:
		s.viewport.SetContent(s.outputContent())
	case 2:
		s.viewport.SetContent(s.workersContent())
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the execution view screen.
func (s ExecutionViewScreen) View() string {
	if s.width == 0 {
		return ""
	}

	var sections []string

	// Tab bar.
	tabBar := s.renderTabBar()
	sections = append(sections, tabBar)

	// Execution info header.
	if s.execution != nil {
		statusBadge := s.renderStatusBadge(s.execution.Status)
		header := fmt.Sprintf("Execution #%d  %s  Branch: %s", s.execution.ID, statusBadge, s.execution.ExecBranch)
		sections = append(sections, s.styles.Header.Render(header))
	}

	// Error display.
	if s.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
		sections = append(sections, errStyle.Render(fmt.Sprintf("Error: %v", s.err)))
	}

	// Running indicator with step detail.
	if s.running {
		runningStyle := lipgloss.NewStyle().Foreground(theme.ColorTextSecondary).Italic(true)
		stepLabel := s.execStepLabel()
		sections = append(sections, runningStyle.Render(stepLabel))
	}

	// Main content viewport (content is already set via updateViewportContent in Update).
	sections = append(sections, s.viewport.View())

	// Footer.
	footer := s.renderFooter()
	sections = append(sections, footer)

	return s.styles.Panel.Width(s.width - 2).Render(strings.Join(sections, "\n\n"))
}

func (s ExecutionViewScreen) execStepLabel() string {
	switch s.execStep {
	case execStepBossPlan:
		return "Running Boss plan phase..."
	case execStepRunningWorkers:
		if s.bossPlan != nil && len(s.bossPlan.NeedsWorkers) > 0 {
			return fmt.Sprintf("Running worker %d/%d...",
				s.currentWorker+1, len(s.bossPlan.NeedsWorkers))
		}
		return "Running workers..."
	case execStepBossSummary:
		return "Running Boss summary phase..."
	case execStepDone:
		return "Execution complete."
	default:
		return "Execution in progress..."
	}
}

func (s ExecutionViewScreen) renderTabBar() string {
	tabs := []string{"Overview", "Output", "Workers"}
	var rendered []string
	for i, tab := range tabs {
		label := fmt.Sprintf(" %s ", tab)
		if i == s.tab {
			rendered = append(rendered, s.styles.TabActive.Render(label))
		} else {
			rendered = append(rendered, s.styles.TabInactive.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (s ExecutionViewScreen) renderStatusBadge(status string) string {
	switch status {
	case db.StatusRunning:
		return s.styles.BadgeRunning.Render(" RUNNING ")
	case db.StatusCompleted:
		return s.styles.BadgeCompleted.Render(" COMPLETED ")
	case db.StatusFailed:
		return s.styles.BadgeFailed.Render(" FAILED ")
	case db.StatusInterrupted:
		return s.styles.BadgeInterrupted.Render(" INTERRUPTED ")
	case db.StatusDiffReview:
		return s.styles.BadgeCompleted.Render(" DIFF REVIEW ")
	case db.StatusPending:
		return s.styles.TabInactive.Render(" PENDING ")
	default:
		return s.styles.TabInactive.Render(fmt.Sprintf(" %s ", strings.ToUpper(status)))
	}
}

func (s ExecutionViewScreen) overviewContent() string {
	var lines []string

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorPrimary)
	valueStyle := lipgloss.NewStyle().Foreground(theme.ColorTextPrimary)

	if s.task != nil {
		lines = append(lines, labelStyle.Render("Task: ")+valueStyle.Render(s.task.Title))
		lines = append(lines, labelStyle.Render("Prompt: ")+valueStyle.Render(s.task.Prompt))
		lines = append(lines, labelStyle.Render("Complexity: ")+valueStyle.Render(s.task.Complexity))
		lines = append(lines, labelStyle.Render("Mode: ")+valueStyle.Render(s.task.Mode))
		lines = append(lines, "")
	}

	if s.execution != nil {
		lines = append(lines, labelStyle.Render("Execution ID: ")+valueStyle.Render(fmt.Sprintf("%d", s.execution.ID)))
		lines = append(lines, labelStyle.Render("Base Branch: ")+valueStyle.Render(s.execution.BaseBranch))
		lines = append(lines, labelStyle.Render("Exec Branch: ")+valueStyle.Render(s.execution.ExecBranch))
		lines = append(lines, labelStyle.Render("Worktree: ")+valueStyle.Render(s.execution.WorktreePath))
		lines = append(lines, labelStyle.Render("Status: ")+valueStyle.Render(s.execution.Status))

		if s.execution.StartedAt != nil {
			lines = append(lines, labelStyle.Render("Started: ")+valueStyle.Render(s.execution.StartedAt.Format("2006-01-02 15:04:05")))
		}
		if s.execution.FinishedAt != nil {
			lines = append(lines, labelStyle.Render("Finished: ")+valueStyle.Render(s.execution.FinishedAt.Format("2006-01-02 15:04:05")))
		}
		if s.execution.StartedAt != nil && s.execution.FinishedAt != nil {
			duration := s.execution.FinishedAt.Sub(*s.execution.StartedAt)
			lines = append(lines, labelStyle.Render("Duration: ")+valueStyle.Render(duration.String()))
		}
	}

	if len(lines) == 0 {
		lines = append(lines, "No execution data available.")
	}

	return strings.Join(lines, "\n")
}

func (s ExecutionViewScreen) outputContent() string {
	if len(s.outputLines) == 0 {
		if s.running {
			return lipgloss.NewStyle().Foreground(theme.ColorTextSecondary).Italic(true).
				Render("Waiting for output...")
		}
		return "No output captured."
	}
	return strings.Join(s.outputLines, "\n")
}

func (s ExecutionViewScreen) workersContent() string {
	if len(s.agentRuns) == 0 {
		if s.running {
			return lipgloss.NewStyle().Foreground(theme.ColorTextSecondary).Italic(true).
				Render("Workers will appear here as they complete...")
		}
		return "No worker runs recorded."
	}

	var lines []string

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorPrimary)
	roleStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTextPrimary)

	for i, run := range s.agentRuns {
		// Outcome badge.
		var badge string
		switch run.Outcome {
		case db.OutcomeSuccess:
			badge = s.styles.BadgeCompleted.Render(" SUCCESS ")
		case db.OutcomePartial:
			badge = s.styles.BadgeInterrupted.Render(" PARTIAL ")
		case db.OutcomeFailed:
			badge = s.styles.BadgeFailed.Render(" FAILED ")
		default:
			badge = s.styles.TabInactive.Render(fmt.Sprintf(" %s ", strings.ToUpper(run.Outcome)))
		}

		header := fmt.Sprintf("%d. %s  [%s]  %s", i+1, roleStyle.Render(run.Role), run.AgentType, badge)
		lines = append(lines, header)

		if run.Summary != "" {
			lines = append(lines, labelStyle.Render("  Summary: ")+run.Summary)
		}
		if run.FilesChanged != "" {
			lines = append(lines, labelStyle.Render("  Files: ")+run.FilesChanged)
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func (s ExecutionViewScreen) renderFooter() string {
	var hints []string
	hints = append(hints, "Tab: switch tabs")
	hints = append(hints, "Esc: back")

	if !s.running && s.execution != nil {
		switch s.execution.Status {
		case db.StatusCompleted, db.StatusFailed, db.StatusDiffReview:
			hints = append(hints, "d: review diff")
		}
		hints = append(hints, "r: refresh")
	}

	return s.styles.StatusBar.Render(strings.Join(hints, " | "))
}
