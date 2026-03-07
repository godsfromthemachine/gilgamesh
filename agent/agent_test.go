package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/godsfromthemachine/gilgamesh/hooks"
	"github.com/godsfromthemachine/gilgamesh/llm"
	"github.com/godsfromthemachine/gilgamesh/session"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockLLMServer returns a test server that cycles through canned SSE responses.
// Each element in responses is a complete SSE stream the mock returns for one
// POST to /chat/completions.  When callCount exceeds len(responses) the last
// response is reused.
func mockLLMServer(responses ...string) *httptest.Server {
	callCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		idx := callCount
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		callCount++
		fmt.Fprint(w, responses[idx])
	}))
}

// sseText builds an SSE stream that returns text content and then stops.
func sseText(content string) string {
	return fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"+
			"data: [DONE]\n\n",
		content,
	)
}

// sseToolCall builds an SSE stream that returns a single tool call.
func sseToolCall(id, name, args string) string {
	return fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":%q,\"type\":\"function\",\"function\":{\"name\":%q,\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":%q}}]},\"finish_reason\":null}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"+
			"data: [DONE]\n\n",
		id, name, args,
	)
}

// newTestAgent creates an agent pointed at the given mock server URL.
// It uses a nil-safe session logger (no file) and an empty hook registry.
func newTestAgent(serverURL string) *Agent {
	client := llm.NewClient(serverURL, "test-key", "test-model")
	return New(client, &hooks.Registry{}, &session.Logger{}, nil, nil)
}

// ---------------------------------------------------------------------------
// 1. TestNewAgent
// ---------------------------------------------------------------------------

func TestNewAgent(t *testing.T) {
	srv := mockLLMServer(sseText("hi"))
	defer srv.Close()

	a := newTestAgent(srv.URL)

	if a == nil {
		t.Fatal("New returned nil")
	}
	if a.registry == nil {
		t.Fatal("registry is nil")
	}
	if len(a.history) != 1 {
		t.Fatalf("history length = %d, want 1 (system prompt)", len(a.history))
	}
	if a.history[0].Role != "system" {
		t.Errorf("history[0].Role = %q, want system", a.history[0].Role)
	}
	if a.history[0].Content == "" {
		t.Error("system prompt content is empty")
	}
}

// ---------------------------------------------------------------------------
// 2. TestClearHistory
// ---------------------------------------------------------------------------

func TestClearHistory(t *testing.T) {
	srv := mockLLMServer(sseText("hi"))
	defer srv.Close()

	a := newTestAgent(srv.URL)

	// Add some history
	a.history = append(a.history, llm.Message{Role: "user", Content: "hello"})
	a.history = append(a.history, llm.Message{Role: "assistant", Content: "world"})

	if len(a.history) != 3 {
		t.Fatalf("history length before clear = %d, want 3", len(a.history))
	}

	a.ClearHistory()

	if len(a.history) != 1 {
		t.Fatalf("history length after clear = %d, want 1", len(a.history))
	}
	if a.history[0].Role != "system" {
		t.Errorf("history[0].Role = %q, want system", a.history[0].Role)
	}
}

// ---------------------------------------------------------------------------
// 3. TestEstimateTokens
// ---------------------------------------------------------------------------

func TestEstimateTokens(t *testing.T) {
	srv := mockLLMServer(sseText("hi"))
	defer srv.Close()

	a := newTestAgent(srv.URL)

	// The agent starts with the system prompt. Add a known user message.
	a.history = append(a.history, llm.Message{Role: "user", Content: strings.Repeat("a", 400)})

	tokens := a.EstimateTokens()

	// system prompt contributes len(prompt)/4, user message contributes 400/4=100
	systemTokens := len(a.history[0].Content) / 4
	expected := systemTokens + 100

	if tokens != expected {
		t.Errorf("EstimateTokens = %d, want %d", tokens, expected)
	}
}

// ---------------------------------------------------------------------------
// 4. TestRegistry
// ---------------------------------------------------------------------------

func TestRegistry(t *testing.T) {
	srv := mockLLMServer(sseText("hi"))
	defer srv.Close()

	a := newTestAgent(srv.URL)
	reg := a.Registry()

	if reg == nil {
		t.Fatal("Registry() returned nil")
	}

	defs := reg.Definitions()
	if len(defs) != 7 {
		names := make([]string, len(defs))
		for i, d := range defs {
			names[i] = d.Function.Name
		}
		t.Fatalf("registry has %d tools (want 7): %v", len(defs), names)
	}
}

// ---------------------------------------------------------------------------
// 5. TestSetClient
// ---------------------------------------------------------------------------

func TestSetClient(t *testing.T) {
	srv1 := mockLLMServer(sseText("from server 1"))
	defer srv1.Close()
	srv2 := mockLLMServer(sseText("from server 2"))
	defer srv2.Close()

	a := newTestAgent(srv1.URL)
	newClient := llm.NewClient(srv2.URL, "k2", "m2")

	a.SetClient(newClient)

	if a.client != newClient {
		t.Error("SetClient did not replace the client")
	}

	// Verify it actually uses the new server.
	err := a.Run("test")
	if err != nil {
		t.Fatalf("Run after SetClient: %v", err)
	}

	// The last assistant message should contain content from server 2.
	lastMsg := a.history[len(a.history)-1]
	if lastMsg.Role != "assistant" {
		t.Fatalf("last message role = %q, want assistant", lastMsg.Role)
	}
	if lastMsg.Content != "from server 2" {
		t.Errorf("last message content = %q, want 'from server 2'", lastMsg.Content)
	}
}

// ---------------------------------------------------------------------------
// 6. TestBuildSystemPrompt
// ---------------------------------------------------------------------------

func TestBuildSystemPrompt(t *testing.T) {
	prompt := SystemPrompt()

	if prompt == "" {
		t.Fatal("SystemPrompt returned empty string")
	}

	lower := strings.ToLower(prompt)
	if !strings.Contains(lower, "test") {
		t.Error("system prompt does not contain 'test'")
	}
	if !strings.Contains(lower, "test-driven") {
		t.Error("system prompt does not contain 'test-driven'")
	}
}

// ---------------------------------------------------------------------------
// 7. TestRunTextResponse
// ---------------------------------------------------------------------------

func TestRunTextResponse(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	srv := mockLLMServer(sse)
	defer srv.Close()

	a := newTestAgent(srv.URL)
	err := a.Run("say hello")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// History should be: system, user, assistant
	if len(a.history) != 3 {
		t.Fatalf("history length = %d, want 3", len(a.history))
	}
	if a.history[1].Role != "user" {
		t.Errorf("history[1].Role = %q, want user", a.history[1].Role)
	}
	if a.history[1].Content != "say hello" {
		t.Errorf("history[1].Content = %q, want 'say hello'", a.history[1].Content)
	}
	if a.history[2].Role != "assistant" {
		t.Errorf("history[2].Role = %q, want assistant", a.history[2].Role)
	}
	if a.history[2].Content != "Hello world" {
		t.Errorf("history[2].Content = %q, want 'Hello world'", a.history[2].Content)
	}
}

// ---------------------------------------------------------------------------
// 8. TestRunWithToolCall
// ---------------------------------------------------------------------------

func TestRunWithToolCall(t *testing.T) {
	// First response: a tool call for glob with pattern "*.go"
	toolResp := sseToolCall("call_1", "glob", `{"pattern":"*.go"}`)
	// Second response: text after processing the tool result
	textResp := sseText("Found some Go files.")

	srv := mockLLMServer(toolResp, textResp)
	defer srv.Close()

	a := newTestAgent(srv.URL)
	err := a.Run("list go files")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// The history should contain: system, user, assistant(tool_call), tool(result), assistant(text)
	if len(a.history) < 5 {
		t.Fatalf("history length = %d, want at least 5", len(a.history))
	}

	// Check tool call was in assistant message
	assistantWithTool := a.history[2]
	if assistantWithTool.Role != "assistant" {
		t.Errorf("history[2].Role = %q, want assistant", assistantWithTool.Role)
	}
	if len(assistantWithTool.ToolCalls) == 0 {
		t.Fatal("assistant message has no tool calls")
	}
	if assistantWithTool.ToolCalls[0].Function.Name != "glob" {
		t.Errorf("tool call name = %q, want glob", assistantWithTool.ToolCalls[0].Function.Name)
	}

	// Check tool result message
	toolResultMsg := a.history[3]
	if toolResultMsg.Role != "tool" {
		t.Errorf("history[3].Role = %q, want tool", toolResultMsg.Role)
	}
	if toolResultMsg.ToolCallID != "call_1" {
		t.Errorf("tool result ToolCallID = %q, want call_1", toolResultMsg.ToolCallID)
	}

	// Check final assistant text
	finalAssistant := a.history[4]
	if finalAssistant.Role != "assistant" {
		t.Errorf("history[4].Role = %q, want assistant", finalAssistant.Role)
	}
	if finalAssistant.Content != "Found some Go files." {
		t.Errorf("final assistant content = %q, want 'Found some Go files.'", finalAssistant.Content)
	}
}

// ---------------------------------------------------------------------------
// 9. TestLoopDetection
// ---------------------------------------------------------------------------

func TestLoopDetection(t *testing.T) {
	// The server always returns the same tool call. After the second call with
	// the same name+args, loop detection should kick in. The agent then injects
	// a "stop repeating" user message and makes one more request, which we
	// make return text.
	toolResp := sseToolCall("call_loop", "glob", `{"pattern":"*.go"}`)
	textResp := sseText("Stopping loop.")

	// The agent will:
	//   call 0 -> toolResp (tool call), executes glob, count["glob:..."] = 1
	//   call 1 -> toolResp (tool call), executes glob, count["glob:..."] = 2 -> LOOP
	//   call 2 -> textResp (forced text response after loop detection)
	srv := mockLLMServer(toolResp, toolResp, textResp)
	defer srv.Close()

	a := newTestAgent(srv.URL)
	err := a.Run("find files")

	// Loop detection returns nil (not an error) after forcing a response.
	if err != nil {
		t.Fatalf("Run returned error %v, want nil (loop detection returns nil)", err)
	}

	// Verify the loop-breaking user message was injected.
	foundLoopMsg := false
	for _, m := range a.history {
		if m.Role == "user" && strings.Contains(m.Content, "repeating tool calls") {
			foundLoopMsg = true
			break
		}
	}
	if !foundLoopMsg {
		t.Error("expected loop-detection user message in history")
	}

	// The final assistant message should contain the forced text.
	last := a.history[len(a.history)-1]
	if last.Role != "assistant" {
		t.Errorf("last message role = %q, want assistant", last.Role)
	}
	if last.Content != "Stopping loop." {
		t.Errorf("last message content = %q, want 'Stopping loop.'", last.Content)
	}
}

// ---------------------------------------------------------------------------
// 10. TestContextCompaction
// ---------------------------------------------------------------------------

func TestContextCompaction(t *testing.T) {
	srv := mockLLMServer(sseText("hi"))
	defer srv.Close()

	a := newTestAgent(srv.URL)

	// Fill history with many large tool result messages to exceed 12000 tokens.
	// Each tool result is 2000 chars = ~500 tokens. We need ~24 of them to
	// exceed the 12000 token threshold.
	bigContent := strings.Repeat("line1\nline2\nline3\nline4\nline5\n", 400) // ~2000 chars, 5+ lines
	for i := 0; i < 30; i++ {
		a.history = append(a.history, llm.Message{
			Role:       "tool",
			Content:    bigContent,
			ToolCallID: fmt.Sprintf("call_%d", i),
		})
	}

	// Verify we are above threshold before compaction.
	tokensBefore := a.EstimateTokens()
	if tokensBefore < compactThreshold {
		t.Fatalf("tokens before compaction = %d, expected >= %d", tokensBefore, compactThreshold)
	}

	a.maybeCompact()

	tokensAfter := a.EstimateTokens()
	if tokensAfter >= tokensBefore {
		t.Errorf("tokens after compaction (%d) should be less than before (%d)", tokensAfter, tokensBefore)
	}

	// Verify that older tool messages were truncated (contain "compacted").
	compactedCount := 0
	for _, m := range a.history {
		if m.Role == "tool" && strings.Contains(m.Content, "...(compacted)") {
			compactedCount++
		}
	}
	if compactedCount == 0 {
		t.Error("expected at least one compacted tool result")
	}

	// The last 4 messages should be untouched (keepTail = 4).
	tail := a.history[len(a.history)-4:]
	for _, m := range tail {
		if strings.Contains(m.Content, "...(compacted)") {
			t.Error("tail messages should not be compacted")
		}
	}
}

// ---------------------------------------------------------------------------
// 11. TestRunWithEvents
// ---------------------------------------------------------------------------

func TestRunWithEvents(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"event\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" test\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	srv := mockLLMServer(sse)
	defer srv.Close()

	a := newTestAgent(srv.URL)

	var events []Event
	err := a.RunWithEvents("hello events", func(e Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("RunWithEvents error: %v", err)
	}

	// We should have received at least two content events.
	contentEvents := 0
	var contentBuf strings.Builder
	for _, e := range events {
		if e.Type == EventContent {
			contentEvents++
			contentBuf.WriteString(e.Content)
		}
	}
	if contentEvents < 2 {
		t.Errorf("content events = %d, want at least 2", contentEvents)
	}
	if contentBuf.String() != "event test" {
		t.Errorf("accumulated content = %q, want 'event test'", contentBuf.String())
	}
}

// ---------------------------------------------------------------------------
// 12. TestTruncate
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"long", "hello world", 5, "hello..."},
		{"empty", "", 5, ""},
		{"zero max", "abc", 0, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.in, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 13. TestEventTypes
// ---------------------------------------------------------------------------

func TestEventTypes(t *testing.T) {
	tests := []struct {
		name string
		et   EventType
		want string
	}{
		{"content", EventContent, "content"},
		{"tool_call", EventToolCall, "tool_call"},
		{"tool_result", EventToolResult, "tool_result"},
		{"error", EventError, "error"},
		{"done", EventDone, "done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.et) != tt.want {
				t.Errorf("EventType %s = %q, want %q", tt.name, string(tt.et), tt.want)
			}
		})
	}
}
