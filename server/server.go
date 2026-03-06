package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/godsfromthemachine/gilgamesh/agent"
	"github.com/godsfromthemachine/gilgamesh/hooks"
	"github.com/godsfromthemachine/gilgamesh/session"
	"github.com/godsfromthemachine/gilgamesh/tools"
)

// Server exposes gilgamesh tools and agent over HTTP.
type Server struct {
	registry *tools.Registry
	agent    *agent.Agent
	hooks    *hooks.Registry
	session  *session.Logger
	version  string
}

func New(registry *tools.Registry, ag *agent.Agent, hookReg *hooks.Registry, sessLog *session.Logger, version string) *Server {
	return &Server{
		registry: registry,
		agent:    ag,
		hooks:    hookReg,
		session:  sessLog,
		version:  version,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/tools", s.handleToolsList)
	mux.HandleFunc("/api/tools/", s.handleToolCall)
	mux.HandleFunc("/api/chat", s.handleChat)
	return mux
}

func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute, // long for SSE streaming
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	done := make(chan error, 1)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nshutting down...\n")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		done <- srv.Shutdown(ctx)
	}()

	fmt.Fprintf(os.Stderr, "gilgamesh HTTP server v%s listening on %s\n", s.version, addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return <-done
}

// GET /api/health

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: s.version})
}

// GET /api/tools

type toolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (s *Server) handleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	toolMap := s.registry.Tools()
	list := make([]toolInfo, 0, len(toolMap))
	for _, t := range toolMap {
		list = append(list, toolInfo{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// POST /api/tools/{name}

type toolCallResponse struct {
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
	Elapsed string `json:"elapsed"`
}

func (s *Server) handleToolCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/tools/")
	if name == "" {
		http.Error(w, "tool name required", http.StatusBadRequest)
		return
	}

	var args json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Pre-hooks
	if s.hooks.HasHooks() {
		preResults := s.hooks.Run(hooks.PreHook, name, args, "")
		for _, pr := range preResults {
			if pr.Err != nil {
				writeJSON(w, http.StatusForbidden, toolCallResponse{Error: "blocked by hook: " + pr.Err.Error()})
				return
			}
		}
	}

	s.session.Log(session.Entry{Type: "tool_call", Tool: name, Args: args})

	start := time.Now()
	result, err := s.registry.Execute(name, args)
	elapsed := time.Since(start)

	s.session.Log(session.Entry{
		Type:     "tool_result",
		Tool:     name,
		Content:  truncStr(result, 500),
		Duration: elapsed,
	})

	// Post-hooks
	if s.hooks.HasHooks() {
		s.hooks.Run(hooks.PostHook, name, args, result)
	}

	if err != nil {
		writeJSON(w, http.StatusOK, toolCallResponse{Error: err.Error(), Elapsed: elapsed.String()})
		return
	}
	writeJSON(w, http.StatusOK, toolCallResponse{Result: result, Elapsed: elapsed.String()})
}

// POST /api/chat (SSE streaming)

type chatRequest struct {
	Message string `json:"message"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	err := s.agent.RunWithEvents(req.Message, func(ev agent.Event) {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	})

	if err != nil {
		errData, _ := json.Marshal(agent.Event{Type: agent.EventError, Error: err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", errData)
		flusher.Flush()
	}

	doneData, _ := json.Marshal(agent.Event{Type: agent.EventDone})
	fmt.Fprintf(w, "data: %s\n\n", doneData)
	flusher.Flush()
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
