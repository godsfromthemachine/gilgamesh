package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/godsfromthemachine/gilgamesh/hooks"
	"github.com/godsfromthemachine/gilgamesh/session"
	"github.com/godsfromthemachine/gilgamesh/tools"
)

func newTestServer() *Server {
	return New(
		tools.NewRegistry(),
		nil, // no agent for tool-only tests
		&hooks.Registry{},
		&session.Logger{},
		"test",
	)
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", srv.handleHealth)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp healthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Version != "test" {
		t.Errorf("version = %q, want test", resp.Version)
	}
}

func TestHealthMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", srv.handleHealth)

	req := httptest.NewRequest("POST", "/api/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestToolsListEndpoint(t *testing.T) {
	srv := newTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tools", srv.handleToolsList)

	req := httptest.NewRequest("GET", "/api/tools", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var list []toolInfo
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(list))
	}

	names := make(map[string]bool)
	for _, tool := range list {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
	}

	for _, expected := range []string{"read", "write", "edit", "bash", "grep", "glob", "test"} {
		if !names[expected] {
			t.Errorf("missing tool: %s", expected)
		}
	}
}

func TestToolCallEndpoint(t *testing.T) {
	srv := newTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tools/", srv.handleToolCall)

	body := strings.NewReader(`{"pattern": "*.go"}`)
	req := httptest.NewRequest("POST", "/api/tools/glob", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp toolCallResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Result == "" {
		t.Error("empty result")
	}
	if resp.Elapsed == "" {
		t.Error("elapsed time not set")
	}
}

func TestToolCallUnknownTool(t *testing.T) {
	srv := newTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tools/", srv.handleToolCall)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest("POST", "/api/tools/nonexistent", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp toolCallResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected error for unknown tool")
	}
}

func TestToolCallInvalidBody(t *testing.T) {
	srv := newTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tools/", srv.handleToolCall)

	body := strings.NewReader(`{invalid json}`)
	req := httptest.NewRequest("POST", "/api/tools/glob", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestToolCallNoName(t *testing.T) {
	srv := newTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tools/", srv.handleToolCall)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest("POST", "/api/tools/", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestToolCallMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tools/", srv.handleToolCall)

	req := httptest.NewRequest("GET", "/api/tools/glob", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestTruncStr(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncStr(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncStr(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
