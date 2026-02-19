package app

import (
	"errors"
	"fmt"

	"bore-tui/internal/config"
	"bore-tui/internal/db"
	"bore-tui/internal/git"
	"bore-tui/internal/logging"
	"bore-tui/internal/process"
)

// App holds all application-wide state for bore-tui.
// It is created once and shared with the TUI layer.
type App struct {
	cluster   *db.Cluster
	db        *db.DB
	config    *config.Config
	state     *config.State
	repo      *git.Repo
	logs      *logging.Manager
	runner    *process.Runner
	scheduler *process.Scheduler
	boreDir   string
	statePath string
}

// Cluster returns the currently open cluster. Nil if none is open.
func (a *App) Cluster() *db.Cluster { return a.cluster }

// DB returns the database connection for the current cluster.
func (a *App) DB() *db.DB { return a.db }

// Config returns the loaded configuration for the current cluster.
func (a *App) Config() *config.Config { return a.config }

// State returns the lightweight UI state.
func (a *App) State() *config.State { return a.state }

// Repo returns the git repository for the current cluster.
func (a *App) Repo() *git.Repo { return a.repo }

// Logs returns the logging manager for the current cluster.
func (a *App) Logs() *logging.Manager { return a.logs }

// Runner returns the Claude CLI process runner.
func (a *App) Runner() *process.Runner { return a.runner }

// Scheduler returns the global worker concurrency scheduler.
func (a *App) Scheduler() *process.Scheduler { return a.scheduler }

// BoreDir returns the path to the .bore/ directory for the current cluster.
func (a *App) BoreDir() string { return a.boreDir }

// New creates a new App instance. It does NOT open a cluster yet.
func New() *App {
	return &App{}
}

// Close cleanly shuts down all resources.
func (a *App) Close() error {
	var errs []error

	if a.state != nil && a.statePath != "" {
		if err := config.SaveState(a.state, a.statePath); err != nil {
			errs = append(errs, fmt.Errorf("app: save state: %w", err))
		}
	}

	if a.logs != nil {
		if err := a.logs.Close(); err != nil {
			errs = append(errs, fmt.Errorf("app: close logs: %w", err))
		}
	}

	if a.db != nil {
		if err := a.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("app: close db: %w", err))
		}
	}

	return errors.Join(errs...)
}
