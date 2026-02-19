package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bore-tui/internal/agents"
	"bore-tui/internal/db"
)

// jsonOK writes v as a JSON 200 response.
func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Allow localhost browser access; no auth on this server.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// response already started; can't write error header
		_ = err
	}
}

// jsonError writes a JSON error response with the given HTTP status code.
func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	// Allow localhost browser access; no auth on this server.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		// response already started; can't write error header
		_ = err
	}
}

// requireDB checks that a cluster is open and returns the DB. If not open it
// writes a 503 response and returns nil.
func (s *Server) requireDB(w http.ResponseWriter) *db.DB {
	d := s.a.DB()
	if d == nil {
		jsonError(w, http.StatusServiceUnavailable, "no cluster open")
		return nil
	}
	return d
}

// parseID parses the "id" path value as an int64.
func parseID(r *http.Request) (int64, error) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("web: parseID: invalid id %q: %w", raw, err)
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// Status & cluster management
// ---------------------------------------------------------------------------

// handleGetStatus returns server status including whether a cluster is open.
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	type clusterDTO struct {
		ID        int64     `json:"id"`
		Name      string    `json:"name"`
		RepoPath  string    `json:"repo_path"`
		RemoteURL *string   `json:"remote_url"`
		CreatedAt time.Time `json:"created_at"`
	}
	type response struct {
		HasCluster bool        `json:"has_cluster"`
		Cluster    *clusterDTO `json:"cluster"`
		Port       int         `json:"port"`
	}

	resp := response{Port: s.port}
	if c := s.a.Cluster(); c != nil {
		resp.HasCluster = true
		resp.Cluster = &clusterDTO{
			ID:        c.ID,
			Name:      c.Name,
			RepoPath:  c.RepoPath,
			RemoteURL: c.RemoteURL,
			CreatedAt: c.CreatedAt,
		}
	}
	jsonOK(w, resp)
}

// handleGetClusters returns the list of known cluster paths.
func (s *Server) handleGetClusters(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	paths := s.a.KnownClusters()
	result := make([]item, 0, len(paths))
	for _, p := range paths {
		result = append(result, item{Path: p, Name: filepath.Base(p)})
	}
	jsonOK(w, result)
}

// handleOpenCluster opens a cluster by path.
func (s *Server) handleOpenCluster(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "web: open cluster: invalid body")
		return
	}
	if req.Path == "" {
		jsonError(w, http.StatusBadRequest, "web: open cluster: path required")
		return
	}
	if err := s.a.OpenCluster(r.Context(), req.Path); err != nil {
		jsonError(w, http.StatusServiceUnavailable, fmt.Sprintf("web: open cluster: %s", err.Error()))
		return
	}
	s.hub.emit("cluster_opened", `{}`)
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

// handleListTasks returns all tasks for the current cluster.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	clusterID := cluster.ID
	tasks, err := d.ListTasks(r.Context(), clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: list tasks: %s", err))
		return
	}
	jsonOK(w, tasks)
}

// handleCreateTask creates a new task in the current cluster.
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	clusterID := cluster.ID

	var body struct {
		Title      string `json:"title"`
		Prompt     string `json:"prompt"`
		Complexity string `json:"complexity"`
		Mode       string `json:"mode"`
		ThreadID   int64  `json:"thread_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("web: create task: decode: %s", err))
		return
	}
	if body.Title == "" || body.Prompt == "" {
		jsonError(w, http.StatusBadRequest, "web: create task: title and prompt are required")
		return
	}

	task, err := d.CreateTask(r.Context(), clusterID, body.ThreadID, body.Title, body.Prompt, body.Complexity, body.Mode)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: create task: %s", err))
		return
	}
	s.hub.emit("tasks_updated", "{}")
	jsonOK(w, task)
}

// handleGetTask returns a single task by ID.
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err := d.GetTask(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: get task: %s", err))
		return
	}
	if task == nil {
		jsonError(w, http.StatusNotFound, "task not found")
		return
	}
	jsonOK(w, task)
}

// ---------------------------------------------------------------------------
// Executions
// ---------------------------------------------------------------------------

// handleListExecutions returns all executions for the current cluster.
func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	clusterID := cluster.ID
	execs, err := d.ListExecutions(r.Context(), clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: list executions: %s", err))
		return
	}
	jsonOK(w, execs)
}

// handleGetExecution returns a single execution by ID.
func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	exec, err := d.GetExecution(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: get execution: %s", err))
		return
	}
	if exec == nil {
		jsonError(w, http.StatusNotFound, "execution not found")
		return
	}
	jsonOK(w, exec)
}

// handleListEvents returns all execution events for a given execution.
func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	events, err := d.ListEvents(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: list events: %s", err))
		return
	}
	jsonOK(w, events)
}

// handleListAgentRuns returns all agent runs for a given execution.
func (s *Server) handleListAgentRuns(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	runs, err := d.GetAgentRuns(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: list agent runs: %s", err))
		return
	}
	jsonOK(w, runs)
}

// ---------------------------------------------------------------------------
// Diff actions
// ---------------------------------------------------------------------------

// handleGetDiff returns the git status and full diff for an execution worktree.
func (s *Server) handleGetDiff(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	repo := s.a.Repo()
	if repo == nil {
		jsonError(w, http.StatusServiceUnavailable, "no repo available")
		return
	}

	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	exec, err := d.GetExecution(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: get diff: get execution: %s", err))
		return
	}
	if exec == nil {
		jsonError(w, http.StatusNotFound, "execution not found")
		return
	}

	status, err := repo.Status(r.Context(), exec.WorktreePath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: get diff: status: %s", err))
		return
	}
	diff, err := repo.DiffAll(r.Context(), exec.WorktreePath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: get diff: diff: %s", err))
		return
	}

	jsonOK(w, map[string]string{
		"status": status,
		"diff":   diff,
	})
}

// handleDiffCommit stages and commits changes in the execution worktree, then
// marks the execution as completed.
func (s *Server) handleDiffCommit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	d := s.requireDB(w)
	if d == nil {
		return
	}
	repo := s.a.Repo()
	if repo == nil {
		jsonError(w, http.StatusServiceUnavailable, "no repo available")
		return
	}

	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("web: diff commit: decode: %s", err))
		return
	}
	if body.Message == "" {
		body.Message = "bore: apply execution changes"
	}

	exec, err := d.GetExecution(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff commit: get execution: %s", err))
		return
	}
	if exec == nil {
		jsonError(w, http.StatusNotFound, "execution not found")
		return
	}

	if err := repo.AddAll(r.Context(), exec.WorktreePath); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff commit: add: %s", err))
		return
	}
	if err := repo.Commit(r.Context(), exec.WorktreePath, body.Message); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff commit: commit: %s", err))
		return
	}
	if err := d.SetExecutionFinished(r.Context(), id, "completed"); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff commit: update status: %s", err))
		return
	}
	_ = d.UpdateTaskStatus(r.Context(), exec.TaskID, db.StatusCompleted)
	s.hub.emit("executions_updated", "{}")
	s.hub.emit("tasks_updated", "{}")
	jsonOK(w, map[string]bool{"ok": true})
}

// handleDiffRevert reverts all changes in the execution worktree and marks
// the execution as interrupted.
func (s *Server) handleDiffRevert(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	d := s.requireDB(w)
	if d == nil {
		return
	}
	repo := s.a.Repo()
	if repo == nil {
		jsonError(w, http.StatusServiceUnavailable, "no repo available")
		return
	}

	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	exec, err := d.GetExecution(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff revert: get execution: %s", err))
		return
	}
	if exec == nil {
		jsonError(w, http.StatusNotFound, "execution not found")
		return
	}

	if err := repo.Revert(r.Context(), exec.WorktreePath, true); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff revert: revert: %s", err))
		return
	}
	if err := d.SetExecutionFinished(r.Context(), id, "interrupted"); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff revert: update status: %s", err))
		return
	}
	_ = d.UpdateTaskStatus(r.Context(), exec.TaskID, db.StatusInterrupted)
	s.hub.emit("executions_updated", "{}")
	s.hub.emit("tasks_updated", "{}")
	jsonOK(w, map[string]bool{"ok": true})
}

// handleDiffMerge stages all changes, commits them, merges the exec branch into
// the base branch, removes the worktree, and marks the execution as completed.
func (s *Server) handleDiffMerge(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	repo := s.a.Repo()
	if repo == nil {
		jsonError(w, http.StatusServiceUnavailable, "no repo available")
		return
	}

	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	exec, err := d.GetExecution(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff merge: get execution: %s", err))
		return
	}
	if exec == nil {
		jsonError(w, http.StatusNotFound, "execution not found")
		return
	}

	if err := repo.AddAll(r.Context(), exec.WorktreePath); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff merge: add: %s", err))
		return
	}
	commitMsg := fmt.Sprintf("bore-tui: execution #%d", exec.ID)
	if err := repo.Commit(r.Context(), exec.WorktreePath, commitMsg); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff merge: commit: %s", err))
		return
	}

	baseBranch := exec.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	if err := repo.MergeInto(r.Context(), baseBranch, exec.ExecBranch); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff merge: merge: %s", err))
		return
	}

	if err := repo.RemoveWorktree(r.Context(), exec.WorktreePath); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: diff merge: remove worktree: %s", err))
		return
	}
	_ = repo.DeleteBranch(r.Context(), exec.ExecBranch)
	_ = repo.PruneWorktrees(r.Context())
	_ = d.UpdateExecutionStatus(r.Context(), exec.ID, db.StatusCompleted)
	_ = d.UpdateTaskStatus(r.Context(), exec.TaskID, db.StatusCompleted)

	s.hub.emit("executions_updated", "{}")
	s.hub.emit("tasks_updated", "{}")
	jsonOK(w, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Merged %s into %s. Worktree cleaned up.", exec.ExecBranch, baseBranch),
	})
}

// ---------------------------------------------------------------------------
// Crews
// ---------------------------------------------------------------------------

// handleListCrews returns all crews for the current cluster.
func (s *Server) handleListCrews(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	clusterID := cluster.ID
	crews, err := d.ListCrews(r.Context(), clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: list crews: %s", err))
		return
	}
	jsonOK(w, crews)
}

// handleCreateCrew creates a new crew in the current cluster.
func (s *Server) handleCreateCrew(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	clusterID := cluster.ID

	var body struct {
		Name            string `json:"name"`
		Objective       string `json:"objective"`
		Constraints     string `json:"constraints"`
		AllowedCommands string `json:"allowed_commands"`
		OwnershipPaths  string `json:"ownership_paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("web: create crew: decode: %s", err))
		return
	}
	if body.Name == "" {
		jsonError(w, http.StatusBadRequest, "web: create crew: name is required")
		return
	}

	crew, err := d.CreateCrew(r.Context(), clusterID, body.Name, body.Objective, body.Constraints, body.AllowedCommands, body.OwnershipPaths)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: create crew: %s", err))
		return
	}
	s.hub.emit("crews_updated", "{}")
	jsonOK(w, crew)
}

// handleUpdateCrew updates an existing crew by ID.
func (s *Server) handleUpdateCrew(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	d := s.requireDB(w)
	if d == nil {
		return
	}

	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Name            string `json:"name"`
		Objective       string `json:"objective"`
		Constraints     string `json:"constraints"`
		AllowedCommands string `json:"allowed_commands"`
		OwnershipPaths  string `json:"ownership_paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("web: update crew: decode: %s", err))
		return
	}

	crew, err := d.GetCrew(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: update crew: get: %s", err))
		return
	}
	if crew == nil {
		jsonError(w, http.StatusNotFound, "web: crew not found")
		return
	}

	crew.Name = body.Name
	crew.Objective = body.Objective
	crew.Constraints = body.Constraints
	crew.AllowedCommands = body.AllowedCommands
	crew.OwnershipPaths = body.OwnershipPaths

	if err := d.UpdateCrew(r.Context(), crew); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: update crew: %s", err))
		return
	}
	s.hub.emit("crews_updated", "{}")
	jsonOK(w, crew)
}

// handleDeleteCrew deletes a crew by ID.
func (s *Server) handleDeleteCrew(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}

	id, err := parseID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := d.DeleteCrew(r.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: delete crew: %s", err))
		return
	}
	s.hub.emit("crews_updated", "{}")
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Threads
// ---------------------------------------------------------------------------

// handleListThreads returns all threads for the current cluster.
func (s *Server) handleListThreads(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	clusterID := cluster.ID
	threads, err := d.ListThreads(r.Context(), clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: list threads: %s", err))
		return
	}
	jsonOK(w, threads)
}

// handleCreateThread creates a new thread in the current cluster.
func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	clusterID := cluster.ID

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("web: create thread: decode: %s", err))
		return
	}
	if body.Name == "" {
		jsonError(w, http.StatusBadRequest, "web: create thread: name is required")
		return
	}

	thread, err := d.CreateThread(r.Context(), clusterID, body.Name, body.Description)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: create thread: %s", err))
		return
	}
	s.hub.emit("threads_updated", "{}")
	jsonOK(w, thread)
}

// ---------------------------------------------------------------------------
// Branches
// ---------------------------------------------------------------------------

// handleListBranches returns all git branches for the current repo.
func (s *Server) handleListBranches(w http.ResponseWriter, r *http.Request) {
	repo := s.a.Repo()
	if repo == nil {
		jsonError(w, http.StatusServiceUnavailable, "no repo available")
		return
	}
	branches, err := repo.ListBranches(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: list branches: %s", err))
		return
	}
	jsonOK(w, map[string][]string{"branches": branches})
}

// ---------------------------------------------------------------------------
// Brain (commander memory)
// ---------------------------------------------------------------------------

// handleGetBrain returns the commander brain content for the current cluster.
func (s *Server) handleGetBrain(w http.ResponseWriter, r *http.Request) {
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	memories, err := d.GetAllMemory(r.Context(), cluster.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: get brain: %s", err.Error()))
		return
	}
	brain := ""
	for _, m := range memories {
		if m.Key == "__brain__" {
			brain = m.Value
			break
		}
	}
	jsonOK(w, map[string]string{"brain": brain})
}

// handleSaveBrain saves the commander brain content for the current cluster.
func (s *Server) handleSaveBrain(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}
	var req struct {
		Brain string `json:"brain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "web: save brain: invalid body")
		return
	}
	if err := d.SetMemory(r.Context(), cluster.ID, "__brain__", req.Brain); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: save brain: %s", err.Error()))
		return
	}
	s.hub.emit("brain_updated", `{}`)
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Commander Chat
// ---------------------------------------------------------------------------

// handleCommanderChat accepts a conversation history and a new user message,
// builds the full Commander context from the DB, calls the Claude CLI, and
// returns the Commander's response as plain text.
func (s *Server) handleCommanderChat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := s.requireDB(w)
	if d == nil {
		return
	}
	cluster := s.a.Cluster()
	if cluster == nil {
		jsonError(w, http.StatusServiceUnavailable, "web: no cluster open")
		return
	}

	var req struct {
		History []agents.ChatMessage `json:"history"`
		Message string               `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("web: commander chat: decode: %s", err))
		return
	}
	if req.Message == "" {
		jsonError(w, http.StatusBadRequest, "web: commander chat: message required")
		return
	}

	clusterID := cluster.ID
	ctx := r.Context()

	brain, err := d.GetAllMemory(ctx, clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: commander chat: brain: %s", err))
		return
	}
	crews, err := d.ListCrews(ctx, clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: commander chat: crews: %s", err))
		return
	}
	threads, err := d.ListThreads(ctx, clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: commander chat: threads: %s", err))
		return
	}
	lessons, err := d.ListAllLessons(ctx, clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: commander chat: lessons: %s", err))
		return
	}
	taskHistory, err := d.ListTaskHistories(ctx, clusterID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: commander chat: task history: %s", err))
		return
	}

	var pastRuns []db.AgentRun
	if execs, err := d.ListExecutions(ctx, clusterID); err == nil {
		limit := 5
		if len(execs) < limit {
			limit = len(execs)
		}
		for _, ex := range execs[:limit] {
			if runs, err := d.GetAgentRuns(ctx, ex.ID); err == nil {
				pastRuns = append(pastRuns, runs...)
			}
		}
	}

	cmdCtx := agents.CommanderContext{
		Brain:       brain,
		Crews:       crews,
		Threads:     threads,
		TaskHistory: taskHistory,
		PastRuns:    pastRuns,
		Lessons:     lessons,
	}

	systemPrompt := agents.BuildCommanderChatSystemPrompt(cmdCtx)
	userMsg := agents.BuildCommanderChatMessage(req.History, req.Message)
	fullPrompt := systemPrompt + "\n\n---\n\n" + userMsg

	repo := s.a.Repo()
	workDir := "."
	if repo != nil && repo.Path != "" {
		workDir = repo.Path
	}

	result := s.a.Runner().Run(context.Background(), workDir, fullPrompt, nil, nil, nil)
	if result.Err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("web: commander chat: claude: %s", result.Err))
		return
	}

	response := strings.TrimSpace(result.Stdout)
	if response == "" {
		response = "(no response)"
	}
	jsonOK(w, map[string]string{"response": response})
}
