package db

import "time"

// ---------------------------------------------------------------------------
// Complexity constants
// ---------------------------------------------------------------------------

const (
	ComplexityBasic   = "basic"
	ComplexityMedium  = "medium"
	ComplexityComplex = "complex"
)

// validComplexities is the set of allowed complexity values.
var validComplexities = map[string]bool{
	ComplexityBasic:   true,
	ComplexityMedium:  true,
	ComplexityComplex: true,
}

// ValidComplexity reports whether s is an allowed complexity value.
func ValidComplexity(s string) bool { return validComplexities[s] }

// ---------------------------------------------------------------------------
// Mode constants
// ---------------------------------------------------------------------------

const (
	ModeJustGetItDone  = "just_get_it_done"
	ModeAlertWithIssues = "alert_with_issues"
)

// validModes is the set of allowed mode values.
var validModes = map[string]bool{
	ModeJustGetItDone:   true,
	ModeAlertWithIssues: true,
}

// ValidMode reports whether s is an allowed mode value.
func ValidMode(s string) bool { return validModes[s] }

// ---------------------------------------------------------------------------
// Task / Execution status constants
// ---------------------------------------------------------------------------

const (
	StatusPending    = "pending"
	StatusReview     = "review"
	StatusRunning    = "running"
	StatusDiffReview = "diff_review"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusInterrupted = "interrupted"
)

// validTaskStatuses is the set of allowed task status values.
var validTaskStatuses = map[string]bool{
	StatusPending:     true,
	StatusReview:      true,
	StatusRunning:     true,
	StatusDiffReview:  true,
	StatusCompleted:   true,
	StatusFailed:      true,
	StatusInterrupted: true,
}

// ValidTaskStatus reports whether s is an allowed task status value.
func ValidTaskStatus(s string) bool { return validTaskStatuses[s] }

// validExecutionStatuses is the set of allowed execution status values.
var validExecutionStatuses = map[string]bool{
	StatusPending:     true,
	StatusReview:      true,
	StatusRunning:     true,
	StatusDiffReview:  true,
	StatusCompleted:   true,
	StatusFailed:      true,
	StatusInterrupted: true,
}

// ValidExecutionStatus reports whether s is an allowed execution status value.
func ValidExecutionStatus(s string) bool { return validExecutionStatuses[s] }

// ---------------------------------------------------------------------------
// Review phase constants
// ---------------------------------------------------------------------------

const (
	PhaseClarification = "clarification"
	PhaseOptions       = "options"
	PhaseSelection     = "selection"
	PhaseBaseBranch    = "base_branch"
)

// ---------------------------------------------------------------------------
// Agent type constants
// ---------------------------------------------------------------------------

const (
	AgentTypeBoss   = "boss"
	AgentTypeWorker = "worker"
)

// ---------------------------------------------------------------------------
// Outcome constants
// ---------------------------------------------------------------------------

const (
	OutcomeSuccess = "success"
	OutcomePartial = "partial"
	OutcomeFailed  = "failed"
)

// ---------------------------------------------------------------------------
// Lesson type constants
// ---------------------------------------------------------------------------

const (
	LessonTypeError   = "error"
	LessonTypePattern = "pattern"
	LessonTypeWarning = "warning"
	LessonTypeNote    = "note"
)

// ---------------------------------------------------------------------------
// Event level constants
// ---------------------------------------------------------------------------

const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Cluster represents a git repository workspace managed by bore-tui.
type Cluster struct {
	ID        int64
	Name      string
	RepoPath  string
	RemoteURL *string
	CreatedAt time.Time
}

// CommanderMemory stores key-value pairs scoped to a cluster for the commander agent.
type CommanderMemory struct {
	ID        int64
	ClusterID int64
	Key       string
	Value     string
	UpdatedAt time.Time
}

// Crew defines a specialized agent team with constraints and ownership rules.
type Crew struct {
	ID              int64
	ClusterID       int64
	Name            string
	Objective       string
	Constraints     string
	AllowedCommands string
	OwnershipPaths  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Thread groups related tasks under a named context within a cluster.
type Thread struct {
	ID          int64
	ClusterID   int64
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Task represents a unit of work assigned within a thread.
type Task struct {
	ID         int64
	ClusterID  int64
	ThreadID   int64
	Title      string
	Prompt     string
	Complexity string // basic, medium, complex
	Mode       string // just_get_it_done, alert_with_issues
	Status     string // pending, review, running, diff_review, completed, failed, interrupted
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TaskReview captures review phase data for a task.
type TaskReview struct {
	ID        int64
	TaskID    int64
	Phase     string // clarification, options, selection, base_branch
	Content   string
	CreatedAt time.Time
}

// Execution tracks a single run of a task, potentially by a crew.
type Execution struct {
	ID           int64
	TaskID       int64
	ClusterID    int64
	CrewID       *int64
	BaseBranch   string
	ExecBranch   string
	WorktreePath string
	Status       string // pending, review, running, diff_review, completed, failed, interrupted
	StartedAt    *time.Time
	FinishedAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ExecutionEvent is a timestamped log entry for an execution.
type ExecutionEvent struct {
	ID          int64
	ExecutionID int64
	Ts          time.Time
	Level       string // debug, info, warn, error
	EventType   string
	Message     string
}

// AgentRun records one agent invocation within an execution.
type AgentRun struct {
	ID           int64
	ExecutionID  int64
	AgentType    string // boss, worker
	Role         string
	Prompt       string
	Summary      string
	Outcome      string // success, partial, failed
	FilesChanged string
	CreatedAt    time.Time
}

// AgentLesson captures a lesson learned during an agent run.
type AgentLesson struct {
	ID          int64
	ExecutionID int64
	AgentType   string // boss, worker
	LessonType  string // error, pattern, warning, note
	Content     string
	CreatedAt   time.Time
}
