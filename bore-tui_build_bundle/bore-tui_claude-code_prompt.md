# Claude Code Master Build Prompt — **bore-tui** (V1)
**Read this entire document before you write any code. Then implement it end-to-end. Do not ask questions; make reasonable defaults exactly as specified below.**

You are building a terminal user interface (TUI) application named **`bore-tui`**. It is a repo-centric, local, persistent, multi-agent orchestration system inspired by "BORE" (Commander → Crews → Tasks → Boss → Workers) and "Beads" (persistent, resumable task threads with strong execution history).

This system will:
- initialize and manage “Clusters” (a Cluster == a Git repo)
- store all persistent state in a repo-local `.bore/` directory with a SQLite DB
- run Claude Code/Claude CLI workers as external processes
- support parallel executions safely via **git worktrees per execution**
- require a **Commander review period** before execution starts
- create a **new git branch per task** off a user-chosen base branch, during review
- enforce **diff review before commit**
- log everything (system, commander, workers, runs) to `.bore/`
- persist **Boss and Worker summarized context** into SQLite so the Commander can reuse it later
- provide a slick dark UI theme (navy + red accents) using Bubble Tea + Lip Gloss
- support mouse interaction in terminal

You MUST implement a complete working V1 with a clean codebase, a minimal but polished TUI, and robust process / git / DB plumbing. Use only local filesystem + system commands. No cloud dependencies.

---

## 0) Tech Choices (MANDATORY)
- Language: **Go**
- TUI: **Bubble Tea**, **Bubbles**, **Lip Gloss**
- SQLite: `database/sql` + either `modernc.org/sqlite` (preferred pure Go) OR `github.com/mattn/go-sqlite3` (allowed if CGO acceptable). Choose `modernc.org/sqlite` by default.
- Git: use `git` CLI via `os/exec` (do not embed a git library)
- Agent execution: run **Claude CLI** (external command) via `os/exec`, capturing stdout/stderr streams.
- JSON: standard library
- Logging: standard library + your own structured wrapper (no heavy logging framework). Implement log rotation yourself (simple size-based).

---

## 1) Repo Layout & Files (MANDATORY)
Create this Go project structure:

```
bore-tui/
  cmd/
    bore-tui/
      main.go
  internal/
    app/            # app state, lifecycle
    tui/            # Bubble Tea model(s), views, keymaps, mouse
    theme/          # Lip Gloss styles, colors
    db/             # sqlite schema, migrations, queries
    git/            # git operations (branching, worktrees, diff)
    agents/         # commander/boss/worker prompt builders, context loading
    process/        # spawning Claude CLI, streaming output
    logging/        # loggers + rotation
    config/         # config parsing, defaults, validation
    util/           # helpers
  assets/           # optional; keep minimal
  go.mod
  README.md
```

When a Cluster is initialized inside a repo, create:

```
repo/
  .bore/
    bore.db
    config.json
    state.json
    logs/
      system.log
      commander.log
      workers/
    runs/
    worktrees/
```

### `.bore/` file rules
- `.bore/bore.db` is the canonical state store.
- `.bore/config.json` is user-editable configuration (see Section 4).
- `.bore/state.json` stores only lightweight UI state (last opened cluster, selected items). DB stores everything important.
- `.bore/logs/*` are rolling operational logs.
- `.bore/runs/<execution_id>/` are immutable run artifacts (metadata + events + patch snapshot).
- `.bore/worktrees/<execution_id>/` is the git worktree path for that execution.

### Git ignore
Create or update `.gitignore` in repo to include:
```
.bore/
```
(We do not commit `.bore/` for V1.)

---

## 2) Core Domain Model (MANDATORY)
### Cluster
- Cluster == Git repo on disk
- Stored with: repo path, created date, name (derived from folder), optional remote URL if cloned

### Commander (logical / reconstructed)
- Commander is NOT a daemon.
- Commander is reconstructed from DB each time the cluster is opened.
- When Commander “speaks/thinks,” call Claude CLI with a strict system prompt and injected DB context.

### Crew (Work Crew)
- A Crew defines objectives for a domain (frontend, backend, devops, etc.)
- Stored in DB (no files)
- Has: name, objective, constraints, allowed tools/commands, optional ownership paths

### Task
- User-submitted prompt text
- Has complexity: `basic | medium | complex`
- Has mode: `just_get_it_done | alert_with_issues`
- Goes through: intake → review → execution → diff_review → done/failed

### Task Thread (“Beads”)
- A “Thread” is a named chain of tasks with shared context and history.
- Commander can suggest continuing a thread.
- Each Task belongs to a Thread (default thread created on first task: “General”)

### Execution
- A concrete run of a Task.
- An Execution has a Boss and 0..N Workers.
- Parallel executions are allowed and expected.

### Boss
- Manager-only agent created per Execution.
- Does NOT code.
- Plans and spawns Workers.
- Aggregates output, produces final summary & lessons.

### Worker
- Ephemeral agent process (Claude CLI) with a narrow role.
- Can modify files inside the execution’s worktree.

---

## 3) Parallel-Safe Git Strategy (MANDATORY)
**Option B (locked): worktree per execution.**

### Review Period branch selection (locked behavior)
During Commander review, the user selects a **base branch** (e.g., `main`, `dev`, etc.). This must be an explicit selection. Then, when the user approves the plan and execution starts:

1. Create a worktree:
   - worktree path: `.bore/worktrees/<execution_id>/`
   - based on the chosen base branch

2. Create an execution branch inside that worktree:
   - branch name format:
     - `bore/<thread-slug>-<task_id>-<short-slug>`
   - where:
     - thread-slug: slugified thread name
     - task_id: integer or UUID short form (choose integer autoincrement for DB; use that)
     - short-slug: slugify first ~6 words of task title/summary

3. Workers modify files ONLY inside the worktree.
4. No branch switching in the main repo ever. The main repo remains stable while parallel worktrees operate.

### Diff review requirement (locked behavior)
After workers finish, before any commit:
- Show git status + git diff in the TUI
- User chooses:
  - Commit changes (to the execution branch)
  - Keep uncommitted (leave branch state as-is)
  - Revert (checkout -- . + clean -fd)
  - Delete execution (remove worktree + delete branch)
- For V1: “merge” is manual (user uses git). Provide a button that prints the exact git commands for merge as guidance.

### Worktree cleanup
- On discard or after delete branch: remove worktree folder and `git worktree prune`.
- Always handle orphaned worktrees on startup (see crash recovery).

---

## 4) Config (MANDATORY)
`.bore/config.json` must be created on cluster init with defaults and be editable in TUI.

### Required config keys
```json
{
  "ui": {
    "theme": "navy_red_dark"
  },
  "agents": {
    "claude_cli_path": "claude",
    "default_model": "",
    "commander_context_limit": 5,
    "max_total_workers": 6,
    "max_workers_basic": 1,
    "max_workers_medium": 2,
    "max_workers_complex": 4
  },
  "git": {
    "worktree_strategy": "worktree",
    "review_required": true,
    "auto_commit": false
  },
  "logging": {
    "level": "info",
    "to_console": true,
    "rotation_mb": 10
  }
}
```

Notes:
- `default_model` may be blank; if blank, use Claude CLI default.
- `max_total_workers` is a global cap across all parallel executions in a cluster.
- Worker spawn logic must respect both per-task limits and global cap (see Section 9).

---

## 5) Logging (MANDATORY, first-class)
Implement **four log categories**, stored in `.bore/`:

1) System log (rolling):
- `.bore/logs/system.log`
- app startup, config, db, git operations, crashes, errors

2) Commander log (rolling):
- `.bore/logs/commander.log`
- commander intake decisions, review options, planning outputs, selection results

3) Worker logs (per worker, rolling per file):
- `.bore/logs/workers/worker-<agent_run_id>.log`
- raw stdout/stderr from that worker process

4) Run artifacts (immutable per execution):
- `.bore/runs/<execution_id>/execution.json`
- `.bore/runs/<execution_id>/events.log`
- `.bore/runs/<execution_id>/diff.patch` (snapshot captured at end before user actions)

### Rotation
Implement size-based rotation for rolling logs:
- if file > `rotation_mb`, move to `.1`, `.2` up to `.5` and truncate new file.
- Keep it simple.

### Live streaming
Worker output must:
- stream live into the TUI
- append to worker log file
- optionally be stored as summarized events in DB (do not store full raw output in DB)

---

## 6) SQLite DB (MANDATORY)
Use a schema and migrations. On init, create tables if missing. Put SQL migration(s) under `internal/db/migrations/` and run them on app open.

Use the exact schema in `bore-tui_schema.sql` (provided separately in this prompt bundle). If you must adjust minor details, keep compatibility and document changes in code comments.

DB is stored at:
- `repo/.bore/bore.db`

DB stores:
- clusters
- commander memory (brain)
- crews
- threads
- tasks
- task reviews (questions/answers/selected option)
- executions
- execution events
- agent_runs (boss and worker summarized context)
- lessons/patterns

---

## 7) TUI Requirements (MANDATORY)
### General UX
- Dark theme, navy primary, red accent (see Section 8)
- 3-pane layout + bottom command bar
- Mouse support enabled: click to select items, scroll logs, click tabs/buttons
- Keyboard navigation must also work fully.

### Screens (minimum)
1) **Home / Cluster Picker**
   - Create cluster
   - Open cluster
   - Recent clusters (from state.json + DB)
2) **Cluster Dashboard**
   - Left: Crews list + Threads list (tabbed)
   - Center: Tasks list (filter by thread/status)
   - Right: Execution detail / Live logs (tabbed: “Summary”, “Logs”, “Diff”)
   - Bottom: input / command bar
3) **Create Cluster Flow**
   - Existing repo path OR git clone URL
   - After selection: init `.bore/`
4) **Commander Builder**
   - Chat-like interface to define commander “brain” (stored in DB)
   - Provide a guided flow: user describes goals; commander proposes brain summary; user approves.
5) **Crew Manager**
   - Create/edit crews: name, objective, constraints, allowed commands, ownership paths
6) **New Task Flow**
   - Enter task prompt
   - Choose complexity
   - Choose mode
   - Select or create thread
   - Then go into Commander Review
7) **Commander Review**
   - Commander asks clarifying questions (if any)
   - Commander proposes 2–3 options
   - User selects an option
   - User selects **base branch**
   - User approves to start execution
8) **Execution View**
   - Shows boss plan
   - Shows workers running and their live output
   - Shows completion summary
9) **Diff Review**
   - Shows git status + diff (paged, scrollable)
   - Buttons: Commit, Keep, Revert, Delete Branch+Worktree
10) **Config Editor**
   - Simple UI to edit config JSON fields (no raw JSON editing in V1; use form-like fields)

### Keybinds (suggested defaults)
- `q` quit (with confirmation if executions running)
- `tab` cycle panes
- `enter` select/open
- `n` new task
- `c` new crew
- `t` new thread
- `r` refresh
- `?` help overlay
- `esc` back

### Non-goals for V1
- No OAuth, no remote services
- No auto-merge
- No embeddings/vector search (store summaries only)

---

## 8) UI Theme (MANDATORY)
Implement a consistent navy/red dark theme with Lip Gloss.

### Color roles (use these exact hex values)
- Background: `#0b0f1a`
- Panels: `#11182a`
- Primary selection/focus (navy): `#1e3a8a`
- Accent (red): `#dc2626`
- Text primary: `#e5e7eb`
- Text secondary: `#9ca3af`
- Border soft: `#24324f`

### Behavior
- Selected items: navy highlight
- Errors/warnings: red
- Diff additions: subtle green (use a muted green, but minimal)
- Diff removals: red
- Avoid rainbow colors; keep it professional and slick.

Centralize all styles in `internal/theme/theme.go` with named styles for:
- panels, headers, list items (normal/selected), buttons (normal/focused/danger), badges (running/failed), diff lines, log lines.

---

## 9) Agent Orchestration (MANDATORY)
### Process model
- Commander, Boss, Workers are “logical agents” represented by prompts + DB context.
- When the system needs agent output, it runs Claude CLI.
- Workers run with their own prompts and operate in the execution worktree directory.

### Global worker cap (MANDATORY)
Because parallel executions exist:
- Enforce `agents.max_total_workers` across all running executions.
- If cap reached, new workers must queue until a slot frees.

Implement a simple in-app scheduler:
- When Boss requests worker spawn, request a “worker slot” from scheduler.
- When worker exits, release slot.
- Scheduler is in-memory; it does not need to survive restarts. On restart, treat running executions as interrupted.

### Complexity → default worker budget
- `basic`: up to `max_workers_basic`
- `medium`: up to `max_workers_medium`
- `complex`: up to `max_workers_complex`
Also clamp by `max_total_workers` availability.

### Modes
- `just_get_it_done`: Boss can proceed with minimal user prompts; still must honor initial review approval.
- `alert_with_issues`: Boss must escalate any blockers and stop early, returning to user with questions.

### Persisted context requirement (locked)
After execution:
- Save **Boss summarized context** and **Worker summarized contexts** into DB so Commander can reuse later.
- Do NOT store raw streaming logs in DB (too large). Store:
  - prompts
  - summaries
  - outcome
  - files changed
  - key errors
  - lessons
Raw logs remain in `.bore/logs/workers/` and `.bore/runs/.../events.log`.

### Commander reuse behavior
During new task intake:
- query DB for similar tasks by:
  - thread match
  - keyword overlap (simple LIKE search on task text + summaries)
  - files changed overlap (if available)
- Load last `commander_context_limit` relevant runs (boss summary + key worker summaries) into Commander prompt context.

---

## 10) Claude CLI Integration (MANDATORY)
### Command invocation
Implement a generic runner that can execute Claude CLI with:
- working directory
- env vars
- stdin prompt
- streaming stdout/stderr

Assume the CLI can be invoked as:
- `claude` with a prompt piped in via stdin

Do NOT hardcode subagent features that may change. Instead, implement a robust “prompt-in / text-out” runner:
- `exec.Command(claudePath)`
- set `cmd.Dir = worktreePath` for workers
- capture stdout/stderr lines
- stream to TUI and logs

### Prompts
All agent prompts must be composed by your code, using templates stored in `internal/agents/templates.go` (or similar). Include:
- strict role
- constraints
- tool usage rules (run commands, edit files, keep changes minimal, etc.)
- output format requirements

Use the “Agent Prompts” file in this bundle as the authoritative content. You will implement these prompts (Commander, Boss, Worker) exactly, injecting dynamic context.

---

## 11) Crash Recovery (MANDATORY)
On app start (and when opening a cluster):
- Detect `.bore/worktrees/*` directories
- Compare against DB executions with status `running`
- If an execution is marked running but no active worker processes exist:
  - mark execution as `interrupted`
  - leave worktree intact
  - show in UI with action buttons:
    - Resume (re-run Boss orchestration from last known plan; V1 can simply restart execution with a new boss run referencing existing branch state)
    - Archive (mark as failed/interrupted, keep artifacts)
    - Delete (remove worktree + delete branch)

For V1, “Resume” can be implemented as:
- create a new execution attempt record linked to same task OR reuse execution id and rerun boss/worker plan
Choose the simpler approach:
- reuse execution id, set status back to `review` and require user confirmation.

---

## 12) Implementation Steps (MANDATORY)
Implement in this order to ensure a working system quickly:

1) Core project skeleton, theme, TUI shell with panes and navigation
2) Config create/load/validate + state.json
3) Cluster init/open:
   - create `.bore/` structure
   - init DB + migrations
4) DB models + queries for clusters, crews, threads, tasks, executions
5) Git module:
   - list branches
   - create worktree
   - create branch
   - diff/status
   - commit/revert/delete
6) Logging module + rotation + log streaming
7) Commander flow:
   - commander brain builder (store in DB)
   - task intake and review generation (via Claude CLI)
8) Boss orchestration:
   - produce plan
   - spawn workers respecting caps
   - capture output
   - finalize summaries + lessons, persist to DB
9) Diff review screen
10) Polish: mouse support, help overlay, config editor

You must deliver a working `bore-tui` binary that can:
- init a repo cluster
- create commander brain + crews
- create a task
- run a worktree execution branch
- stream logs
- show diff
- persist summaries

---

## 13) Acceptance Criteria (MANDATORY)
### Functional
- Can init/open cluster for existing repo or clone URL
- Creates `.bore/` folder and DB properly
- Creates commander brain stored in DB
- Can create crews stored in DB
- Can create threads and tasks
- Commander review includes:
  - clarifying Qs (if needed)
  - option selection
  - base branch selection
- Starting execution:
  - creates worktree
  - creates execution branch
  - spawns boss/workers
  - enforces worker caps
- After execution:
  - shows diff review
  - allows commit/revert/delete
- Persists boss + worker summaries to DB
- Logging works and rotates

### UX
- Dark navy/red theme throughout
- Mouse works for selecting lists and scrolling logs
- No screen flicker; updates are smooth

### Code quality
- Clean modules, no god files
- Errors handled and surfaced in UI
- No panics in normal usage

---

## 14) Output requirements
When you finish:
- Provide a README with:
  - install/build instructions
  - how to init a cluster
  - how to run a task
  - how to review diffs
- Provide a short “Developer Notes” section describing major modules and extension points.

---

## 15) IMPORTANT CONSTRAINTS
- Do not ask questions.
- Do not change the requirements.
- If a requirement is ambiguous, choose the simplest option that matches the spirit of BORE + Beads.
- Implement V1 fully and make it stable.

---

## Bundle files
This prompt comes with supporting files:
- `bore-tui_schema.sql` — SQLite schema (apply as migration 0001)
- `bore-tui_agent_prompts.md` — exact prompt templates and output formats
- `bore-tui_config_reference.md` — config field definitions and defaults

Use them.
