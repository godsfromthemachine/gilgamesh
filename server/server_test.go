package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestHandlerMethod(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}

	// Verify the handler can serve a health request end-to-end
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Handler health status = %d, want 200", w.Code)
	}
	var resp healthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
}

func TestToolCallWithHooks(t *testing.T) {
	// Test with hooks loaded from a temp hooks.json file
	tmpDir := t.TempDir()
	hooksDir := tmpDir + "/.gilgamesh"
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a pre-hook that succeeds (exit 0) and a post-hook
	hooksJSON := `[
		{"tool": "*", "type": "pre", "command": "echo pre-hook-ran"},
		{"tool": "*", "type": "post", "command": "echo post-hook-ran"}
	]`
	if err := os.WriteFile(hooksDir+"/hooks.json", []byte(hooksJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Save current dir, chdir to tmp so hooks.Load() finds the file
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	hookReg := hooks.Load()
	if !hookReg.HasHooks() {
		t.Fatal("expected hooks to be loaded")
	}

	srv := New(
		tools.NewRegistry(),
		nil,
		hookReg,
		&session.Logger{},
		"test",
	)

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
	if resp.Elapsed == "" {
		t.Error("elapsed time not set")
	}
}

func TestToolCallWithBlockingHook(t *testing.T) {
	tmpDir := t.TempDir()
	hooksDir := tmpDir + "/.gilgamesh"
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a pre-hook that fails (exit 1) to block tool execution
	hooksJSON := `[{"tool": "*", "type": "pre", "command": "exit 1"}]`
	if err := os.WriteFile(hooksDir+"/hooks.json", []byte(hooksJSON), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	hookReg := hooks.Load()
	srv := New(tools.NewRegistry(), nil, hookReg, &session.Logger{}, "test")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tools/", srv.handleToolCall)

	body := strings.NewReader(`{"pattern": "*.go"}`)
	req := httptest.NewRequest("POST", "/api/tools/glob", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}

	var resp toolCallResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp.Error, "blocked by hook") {
		t.Errorf("expected 'blocked by hook' error, got %q", resp.Error)
	}
}

func TestChatEndpointMissingMessage(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	body := strings.NewReader(`{"message": ""}`)
	req := httptest.NewRequest("POST", "/api/chat", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestChatEndpointMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	req := httptest.NewRequest("GET", "/api/chat", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestChatEndpointInvalidJSON(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	body := strings.NewReader(`{not valid json}`)
	req := httptest.NewRequest("POST", "/api/chat", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestToolCallSuccessResponse(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	body := strings.NewReader(`{"pattern": "*.go"}`)
	req := httptest.NewRequest("POST", "/api/tools/glob", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Verify JSON structure has "result" and "elapsed" fields
	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := raw["result"]; !ok {
		t.Error("response missing 'result' field")
	}
	if _, ok := raw["elapsed"]; !ok {
		t.Error("response missing 'elapsed' field")
	}
	// On success, "error" should not be present (omitempty)
	if errVal, ok := raw["error"]; ok && errVal != "" {
		t.Errorf("unexpected error field: %v", errVal)
	}
}

func TestToolCallErrorResponse(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	// Call an unknown tool to trigger an error response
	body := strings.NewReader(`{}`)
	req := httptest.NewRequest("POST", "/api/tools/nonexistent", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := raw["error"]; !ok {
		t.Error("response missing 'error' field")
	}
	if _, ok := raw["elapsed"]; !ok {
		t.Error("response missing 'elapsed' field")
	}
	// On error, "result" should not be present (omitempty)
	if resVal, ok := raw["result"]; ok && resVal != "" {
		t.Errorf("unexpected result field on error: %v", resVal)
	}
}

func TestHealthResponseJSON(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Verify exact JSON structure
	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(raw) != 2 {
		t.Errorf("expected 2 fields in health response, got %d", len(raw))
	}
	if raw["status"] != "ok" {
		t.Errorf("status = %v, want \"ok\"", raw["status"])
	}
	if raw["version"] != "test" {
		t.Errorf("version = %v, want \"test\"", raw["version"])
	}

	// Verify Content-Type header
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestToolsListResponseJSON(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	req := httptest.NewRequest("GET", "/api/tools", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Verify the response is a JSON array
	var list []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode response as array: %v", err)
	}

	if len(list) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(list))
	}

	// Verify each tool has the required fields
	for _, tool := range list {
		if _, ok := tool["name"]; !ok {
			t.Error("tool missing 'name' field")
		}
		if _, ok := tool["description"]; !ok {
			t.Error("tool missing 'description' field")
		}
		if _, ok := tool["parameters"]; !ok {
			t.Error("tool missing 'parameters' field")
		}
		// Verify name is a non-empty string
		name, ok := tool["name"].(string)
		if !ok || name == "" {
			t.Errorf("tool name should be non-empty string, got %v", tool["name"])
		}
	}

	// Verify Content-Type header
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestToolsListMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	req := httptest.NewRequest("POST", "/api/tools", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}
