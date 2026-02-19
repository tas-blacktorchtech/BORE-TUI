
# bore-tui

**bore-tui** is a terminal-based AI orchestration system for software repositories.

It provides a local, persistent, repo-centric workflow inspired by:
- BORE architecture (Commander → Crews → Boss → Workers)
- Beads-style persistent execution history
- Claude Code / Claude CLI execution

All project intelligence is stored locally in `.bore/` inside your repository.

---

## Features (V1)

- Repo-based clusters
- Persistent SQLite memory (`.bore/bore.db`)
- Commander with long-term memory
- Crews (Frontend, Backend, etc.)
- Task threads (Beads)
- Review phase before execution
- Git branch per task
- Git **worktree per execution**
- Parallel executions
- Diff review before commit
- Persistent Boss + Worker summaries
- Full logging system
- Dark navy/red TUI
- Mouse support

---

## Requirements

- Go 1.22+
- Git installed
- Claude CLI installed and available in PATH (default command: `claude`)

---

## Build

```
git clone <your-repo>
cd bore-tui
go mod tidy
go build -o bore-tui ./cmd/bore-tui
```

---

## First Run

Inside any project repository:

```
bore-tui
```

### Create Cluster

You will be prompted to:

1. Select existing repo path  
   OR  
2. Git clone a repository  

Then bore-tui will create:

```
repo/
  .bore/
    bore.db
    config.json
    logs/
    runs/
    worktrees/
```

---

## Setup Flow

### 1) Create Commander Brain
You’ll chat with Commander to define:
- Project goals
- Architecture style
- Constraints
- Coding standards

Stored in SQLite.

---

### 2) Create Crews
Example:
- Frontend Crew
- Backend Crew
- DevOps Crew

Each crew has:
- Objective
- Constraints
- Allowed commands
- Ownership paths

---

### 3) Create a Task

Flow:

1. Enter task prompt
2. Choose complexity (basic / medium / complex)
3. Choose mode:
   - just_get_it_done
   - alert_with_issues
4. Choose thread (or create one)

---

### 4) Commander Review

Commander will:

- Ask clarifying questions (if needed)
- Propose execution options
- Suggest crew + worker count

You then:

- Select option
- Select **base branch**
- Approve execution

---

## Execution Behavior

When execution starts:

1. Create worktree  
   `.bore/worktrees/<execution-id>/`
2. Create branch:
   `bore/<thread>-<task-id>`
3. Boss plans work
4. Workers run Claude CLI
5. Output streams live in TUI

Parallel executions are supported.

Global worker limits enforced via config.

---

## Diff Review

After workers finish:

You can:

- Commit changes
- Keep uncommitted
- Revert
- Delete branch + worktree

Merging to main is manual.

---

## Logging

```
.bore/logs/system.log
.bore/logs/commander.log
.bore/logs/workers/
.bore/runs/<execution>/
```

Includes:
- execution.json
- events.log
- diff.patch

---

## Config

Edit:

```
.bore/config.json
```

Controls:
- Worker limits
- Global concurrency
- Logging
- Claude CLI path
- Context limits

---

## Crash Recovery

On startup:

- Interrupted executions detected
- Worktrees preserved
- Options:
  - Resume
  - Archive
  - Delete

---

## Design Philosophy

- Local-first
- Git-native safety
- Persistent memory
- Review before execution
- Parallel-safe
- No cloud dependencies

---

## Future Extensions (not in V1)

- Git auto-merge
- Embeddings / semantic search
- Remote execution
- Web UI
- Plugin system

---

## Developer Notes

Key modules:

- `internal/tui` — UI and navigation
- `internal/db` — SQLite schema + queries
- `internal/git` — branch/worktree/diff
- `internal/process` — Claude execution
- `internal/agents` — prompt construction
- `internal/logging` — rotation + streaming
- `internal/config` — config management
- `internal/theme` — Lip Gloss styles

---

You can now run bore-tui, give it a task, and leave it running.
