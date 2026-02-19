# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o bore-tui ./cmd/bore-tui     # build binary
go build ./...                           # check all packages compile
go vet ./...                             # static analysis
go test ./...                            # run all tests
go test ./internal/db/...                # single package tests
```

Entry point: `cmd/bore-tui/main.go`

## Tech Stack

- **Go 1.22+** with `modernc.org/sqlite` (pure Go, no CGO)
- **TUI**: Bubble Tea + Bubbles + Lip Gloss
- **Git**: CLI via `os/exec`
- **Agent execution**: Claude CLI via `os/exec` with `-p` flag, stdin prompt

## Architecture

### Agent Hierarchy
Commander → Crews → Boss → Workers. Commander does intake/review, Boss plans and delegates, Workers edit code in git worktrees. All are Claude CLI processes with structured JSON output.

### Module Layout
```
cmd/bore-tui/main.go    — Bubble Tea program entry
internal/
  app/       — App struct (central state), cluster init/open, crash recovery
  tui/       — All TUI screens, model.go dispatches to active screen
  theme/     — Lip Gloss styles: const colors + Styles struct via DefaultStyles()
  db/        — SQLite: Open(), migrations, typed queries on unexported conn field
  git/       — Repo struct: branches, worktrees, diff, commit, clone, slug
  agents/    — Prompt builders (Commander/Boss/Worker) + response type parsing
  process/   — Runner (Claude CLI), Scheduler (worker slot semaphore), JSON extraction
  logging/   — Logger with atomic level, size-based rotation; Manager for categories
  config/    — Config + State JSON load/save/validate; shared loadJSON/saveJSON helpers
```

### Data Flow
Task: `intake → commander_review (clarifications → options → brief) → execution (boss_plan → workers → boss_summary) → diff_review → done`

### Cluster Structure (in target repo)
`.bore/` contains: `bore.db`, `config.json`, `state.json`, `logs/`, `runs/`, `worktrees/`

## Code Conventions (enforced across all waves)

- `context.Context` as first parameter on anything doing I/O
- No mutable package-level variables (use functions returning fresh values)
- Error messages prefixed with package name: `fmt.Errorf("process: start: %w", err)`
- `errors.As` for type assertions on errors (not direct `err.(*Type)`)
- Private struct fields with getter methods (App, DB, Runner)
- `errors.Join` for multi-error aggregation
- `0o755` octal format for permissions
- Scanner interface pattern for DRY in DB queries
- `[]any` not `[]interface{}`

## TUI Screen Pattern

Each screen in `internal/tui/` follows one of two conventions:
1. **Pointer receiver**: `Update(msg) tea.Cmd` + `View(width, height) string` — used by home, createCluster, dashboard, commanderBuilder, crewManager, newTask, configEditor
2. **Value receiver**: `Update(msg) (ScreenType, tea.Cmd)` + `View() string` — used by commanderReview, executionView, diffReview (store dimensions internally)

`model.go` dispatches correctly for both patterns. Navigation uses a `screenStack` for proper back navigation.

## Key Specs (read-only reference)
- `bore-tui_build_bundle/bore-tui_claude-code_prompt.md` — master spec
- `bore-tui_build_bundle/bore-tui_schema.sql` — SQLite schema
- `bore-tui_build_bundle/bore-tui_agent_prompts.md` — prompt templates + JSON formats
- `bore-tui_build_bundle/bore-tui_config_reference.md` — config fields
