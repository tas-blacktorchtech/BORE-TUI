// Package web provides the BORE web GUI HTTP server with REST API and SSE.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"bore-tui/internal/app"
)

//go:embed static
var staticFiles embed.FS

const defaultPort = 8742

// Server is the BORE web GUI HTTP server.
type Server struct {
	a    *app.App
	srv  *http.Server
	port int
	hub  *sseHub
}

// New creates a new Server bound to the given App.
func New(a *app.App) *Server {
	return &Server{a: a, hub: newSSEHub()}
}

// Port returns the port the server is listening on (0 if not started).
func (s *Server) Port() int { return s.port }

// URL returns the base URL (e.g., "http://localhost:8742").
func (s *Server) URL() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

// Start binds to a free port starting at defaultPort, starts the HTTP server
// in a background goroutine, and opens the browser. Returns the URL.
func (s *Server) Start(ctx context.Context) (string, error) {
	ln, err := freePort(defaultPort)
	if err != nil {
		return "", fmt.Errorf("web: start: find port: %w", err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.srv = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		// WriteTimeout intentionally 0 — SSE connections must not time out.
	}

	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// error is non-fatal after Stop() is called; log to stderr in debug builds
			_ = err
		}
	}()
	go s.hub.run()

	url := s.URL()
	_ = openBrowser(ctx, url)
	return url, nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("web: stop: %w", err)
	}
	close(s.hub.quit)
	return nil
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(fmt.Sprintf("web: embed static: %v", err))
	}
	mux.Handle("GET /", http.FileServer(http.FS(static)))

	// SSE
	mux.HandleFunc("GET /events", s.handleSSE)

	// Status & cluster management
	mux.HandleFunc("GET /api/status", s.handleGetStatus)
	mux.HandleFunc("GET /api/clusters", s.handleGetClusters)
	mux.HandleFunc("POST /api/clusters/open", s.handleOpenCluster)

	// Tasks
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleGetTask)

	// Executions
	mux.HandleFunc("GET /api/executions", s.handleListExecutions)
	mux.HandleFunc("GET /api/executions/{id}", s.handleGetExecution)
	mux.HandleFunc("GET /api/executions/{id}/events", s.handleListEvents)
	mux.HandleFunc("GET /api/executions/{id}/runs", s.handleListAgentRuns)

	// Diff actions
	mux.HandleFunc("GET /api/diff/{id}", s.handleGetDiff)
	mux.HandleFunc("POST /api/diff/{id}/commit", s.handleDiffCommit)
	mux.HandleFunc("POST /api/diff/{id}/revert", s.handleDiffRevert)
	mux.HandleFunc("POST /api/diff/{id}/merge", s.handleDiffMerge)

	// Crews
	mux.HandleFunc("GET /api/crews", s.handleListCrews)
	mux.HandleFunc("POST /api/crews", s.handleCreateCrew)
	mux.HandleFunc("PUT /api/crews/{id}", s.handleUpdateCrew)
	mux.HandleFunc("DELETE /api/crews/{id}", s.handleDeleteCrew)

	// Threads
	mux.HandleFunc("GET /api/threads", s.handleListThreads)
	mux.HandleFunc("POST /api/threads", s.handleCreateThread)

	// Brain
	mux.HandleFunc("GET /api/brain", s.handleGetBrain)
	mux.HandleFunc("PUT /api/brain", s.handleSaveBrain)

	// Commander chat
	mux.HandleFunc("POST /api/commander/chat", s.handleCommanderChat)

	// Git
	mux.HandleFunc("GET /api/branches", s.handleListBranches)
}

// freePort finds the first available TCP port starting from start and returns
// the bound listener. The caller is responsible for using or closing it.
func freePort(start int) (net.Listener, error) {
	for p := start; p < start+100; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("web: freePort: no free port found in range %d-%d", start, start+100)
}

// openBrowser opens the given URL in the system default browser.
func openBrowser(ctx context.Context, url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}
	go func() { _ = cmd.Run() }()
	return nil
}
