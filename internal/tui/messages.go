package tui

import (
	"bore-tui/internal/agents"
	"bore-tui/internal/db"
)

// Screen identifies which screen the TUI is currently showing.
type Screen int

const (
	ScreenHome             Screen = iota
	ScreenCreateCluster
	ScreenDashboard
	ScreenCommanderBuilder
	ScreenCrewManager
	ScreenNewTask
	ScreenCommanderReview
	ScreenExecutionView
	ScreenDiffReview
	ScreenConfigEditor
)

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

// NavigateMsg tells the main model to switch to a different screen.
// Data carries optional context for the target screen.
type NavigateMsg struct {
	Screen Screen
	Data   any
}

// NavigateBackMsg tells the main model to go back to the previous screen.
type NavigateBackMsg struct{}

// ---------------------------------------------------------------------------
// Status and errors
// ---------------------------------------------------------------------------

// ErrorMsg carries an error to be displayed in the status bar.
type ErrorMsg struct{ Err error }

// StatusMsg carries a status string to be displayed in the status bar.
type StatusMsg string

// ---------------------------------------------------------------------------
// Cluster lifecycle
// ---------------------------------------------------------------------------

// ClusterOpenedMsg is sent after a cluster has been successfully opened.
type ClusterOpenedMsg struct{}

// ClusterInitDoneMsg is sent after a new cluster has been initialized.
type ClusterInitDoneMsg struct{}

// ---------------------------------------------------------------------------
// Commander flow
// ---------------------------------------------------------------------------

// ClarificationsReceivedMsg carries the Commander's clarification questions.
type ClarificationsReceivedMsg struct {
	Response agents.ClarificationsResponse
}

// OptionsReceivedMsg carries the Commander's proposed execution options.
type OptionsReceivedMsg struct {
	Response agents.OptionsResponse
}

// BriefReceivedMsg carries the Commander's final execution brief.
type BriefReceivedMsg struct {
	Response agents.ExecutionBrief
}

// ---------------------------------------------------------------------------
// Execution flow
// ---------------------------------------------------------------------------

// ExecutionStartedMsg is sent when an execution begins running.
type ExecutionStartedMsg struct {
	Execution *db.Execution
}

// WorkerOutputMsg carries a single line of output from a running worker.
type WorkerOutputMsg struct {
	RunID    int64
	Line     string
	IsStderr bool
}

// ExecutionDoneMsg is sent when an execution finishes.
type ExecutionDoneMsg struct {
	ExecutionID int64
	Status      string
}

// ---------------------------------------------------------------------------
// Data refresh
// ---------------------------------------------------------------------------

// ClustersLoadedMsg carries a freshly loaded list of clusters.
type ClustersLoadedMsg struct{ Clusters []db.Cluster }

// TasksLoadedMsg carries a freshly loaded list of tasks.
type TasksLoadedMsg struct{ Tasks []db.Task }

// ExecutionsLoadedMsg carries a freshly loaded list of executions.
type ExecutionsLoadedMsg struct{ Executions []db.Execution }

// CrewsLoadedMsg carries a freshly loaded list of crews.
type CrewsLoadedMsg struct{ Crews []db.Crew }

// ThreadsLoadedMsg carries a freshly loaded list of threads.
type ThreadsLoadedMsg struct{ Threads []db.Thread }

// BranchesLoadedMsg carries a freshly loaded list of branch names.
type BranchesLoadedMsg struct{ Branches []string }

// DiffLoadedMsg carries git status and diff output for review.
type DiffLoadedMsg struct {
	Status string
	Diff   string
}

// ---------------------------------------------------------------------------
// Tick for animations
// ---------------------------------------------------------------------------

// TickMsg is sent on a periodic interval for spinner/animation updates.
type TickMsg struct{}
