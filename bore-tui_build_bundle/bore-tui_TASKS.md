
# bore-tui Implementation Checklist (Strict Build Order)

This file defines the exact order Claude should follow.
Do NOT skip steps.

---

## Phase 1 — Project Skeleton

- [ ] Create Go module
- [ ] Create folder structure:
  - cmd/bore-tui
  - internal/app
  - internal/tui
  - internal/theme
  - internal/db
  - internal/git
  - internal/process
  - internal/agents
  - internal/logging
  - internal/config
  - internal/util
- [ ] Basic main.go
- [ ] Start Bubble Tea app shell

---

## Phase 2 — Theme + Base UI

- [ ] Implement navy/red theme (Lip Gloss)
- [ ] Layout:
  - Left pane
  - Center pane
  - Right pane
  - Bottom command bar
- [ ] Mouse enabled
- [ ] Keyboard navigation
- [ ] Help overlay

---

## Phase 3 — Config System

- [ ] Create `.bore/config.json` defaults
- [ ] Load + validate config
- [ ] Config editor UI

---

## Phase 4 — Cluster Init

- [ ] Select repo path OR git clone
- [ ] Create `.bore/` structure
- [ ] Create DB
- [ ] Run migrations
- [ ] Update `.gitignore`

---

## Phase 5 — Database Layer

- [ ] Implement schema from bore-tui_schema.sql
- [ ] Create data access methods for:
  - clusters
  - crews
  - threads
  - tasks
  - executions
  - agent_runs
  - events

---

## Phase 6 — Git Module

- [ ] List branches
- [ ] Create worktree
- [ ] Create execution branch
- [ ] Git status
- [ ] Git diff
- [ ] Commit
- [ ] Revert
- [ ] Delete worktree + branch
- [ ] Worktree cleanup on startup

---

## Phase 7 — Logging

- [ ] system.log
- [ ] commander.log
- [ ] worker logs
- [ ] rotation
- [ ] execution events
- [ ] run artifacts

---

## Phase 8 — Commander

- [ ] Commander brain builder UI
- [ ] Store brain in DB
- [ ] Task intake
- [ ] Clarifications
- [ ] Options generation
- [ ] Base branch selection
- [ ] Execution brief

---

## Phase 9 — Execution Engine

- [ ] Global worker scheduler
- [ ] Respect max_total_workers
- [ ] Complexity-based limits
- [ ] Boss orchestration
- [ ] Worker spawn via Claude CLI
- [ ] Stream output to UI + logs
- [ ] Persist summaries

---

## Phase 10 — Diff Review

- [ ] Show status
- [ ] Show diff
- [ ] Actions:
  - Commit
  - Keep
  - Revert
  - Delete

---

## Phase 11 — Parallel Execution Support

- [ ] Multiple running executions
- [ ] Independent worktrees
- [ ] Execution status tracking
- [ ] Interrupted detection
- [ ] Resume / Archive / Delete options

---

## Phase 12 — Final Polish

- [ ] Smooth UI updates
- [ ] Error handling (no panics)
- [ ] README accuracy
- [ ] Build works cleanly

---

## Completion Criteria

System must be able to:

1. Initialize cluster
2. Create commander + crews
3. Create task
4. Review plan
5. Create branch + worktree
6. Run Claude workers
7. Stream output
8. Show diff
9. Persist context
10. Run multiple executions safely
