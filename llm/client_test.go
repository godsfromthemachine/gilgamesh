package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- NewClient tests ---

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:8080/v1", "sk-test", "gpt-4")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.endpoint != "http://localhost:8080/v1" {
		t.Errorf("endpoint = %q, want http://localhost:8080/v1", c.endpoint)
	}
	if c.apiKey != "sk-test" {
		t.Errorf("apiKey = %q, want sk-test", c.apiKey)
	}
	if c.model != "gpt-4" {
		t.Errorf("model = %q, want gpt-4", c.model)
	}
	if c.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	c := NewClient("http://localhost:8080/v1/", "key", "model")
	if c.endpoint != "http://localhost:8080/v1" {
		t.Errorf("endpoint = %q, want trailing slash trimmed", c.endpoint)
	}
}

func TestNewClientEmptyValues(t *testing.T) {
	c := NewClient("", "", "")
	if c == nil {
		t.Fatal("NewClient returned nil for empty values")
	}
	if c.endpoint != "" {
		t.Errorf("endpoint = %q, want empty", c.endpoint)
	}
}

// --- Request building tests ---

func TestStreamChatRequestFormat(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		capturedBody, _ = io.ReadAll(r.Body)
		// Return a minimal valid SSE stream
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-api-key", "test-model")
	msgs := []Message{{Role: "user", Content: "hello"}}
	err := c.StreamChat(msgs, nil, func(d StreamDelta) {})
	if err != nil {
		t.Fatalf("StreamChat error: %v", err)
	}

	// Verify HTTP method
	if capturedReq.Method != "POST" {
		t.Errorf("method = %q, want POST", capturedReq.Method)
	}

	// Verify URL path
	if capturedReq.URL.Path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", capturedReq.URL.Path)
	}

	// Verify headers
	if ct := capturedReq.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if auth := capturedReq.Header.Get("Authorization"); auth != "Bearer test-api-key" {
		t.Errorf("Authorization = %q, want Bearer test-api-key", auth)
	}

	// Verify body
	var reqBody ChatRequest
	if err := json.Unmarshal(capturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if reqBody.Model != "test-model" {
		t.Errorf("body model = %q, want test-model", reqBody.Model)
	}
	if !reqBody.Stream {
		t.Error("body stream = false, want true")
	}
	if len(reqBody.Messages) != 1 {
		t.Fatalf("body messages len = %d, want 1", len(reqBody.Messages))
	}
	if reqBody.Messages[0].Role != "user" {
		t.Errorf("message role = %q, want user", reqBody.Messages[0].Role)
	}
	if reqBody.Messages[0].Content != "hello" {
		t.Errorf("message content = %q, want hello", reqBody.Messages[0].Content)
	}
	if reqBody.Tools != nil {
		t.Errorf("tools should be nil/omitted when no tools provided, got %v", reqBody.Tools)
	}
}

func TestStreamChatRequestWithTools(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "model")
	tools := []ToolDef{
		{
			Type: "function",
			Function: ToolDefFunc{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	}

	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, tools, func(d StreamDelta) {})
	if err != nil {
		t.Fatalf("StreamChat error: %v", err)
	}

	var reqBody ChatRequest
	if err := json.Unmarshal(capturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(reqBody.Tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(reqBody.Tools))
	}
	if reqBody.Tools[0].Function.Name != "read_file" {
		t.Errorf("tool name = %q, want read_file", reqBody.Tools[0].Function.Name)
	}
}

// --- SSE stream parsing tests ---

func TestStreamChatContentDeltas(t *testing.T) {
	sseData := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", "m")
	var deltas []StreamDelta
	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, nil, func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("StreamChat error: %v", err)
	}

	// Expect: "Hello" delta, " world" delta, finish delta (Done=true), DONE delta (Done=true)
	if len(deltas) < 3 {
		t.Fatalf("got %d deltas, want at least 3", len(deltas))
	}
	if deltas[0].Content != "Hello" {
		t.Errorf("delta[0].Content = %q, want Hello", deltas[0].Content)
	}
	if deltas[1].Content != " world" {
		t.Errorf("delta[1].Content = %q, want ' world'", deltas[1].Content)
	}

	// The finish_reason=stop chunk should set Done
	foundDone := false
	for _, d := range deltas {
		if d.Done {
			foundDone = true
			break
		}
	}
	if !foundDone {
		t.Error("no delta had Done=true")
	}
}

func TestStreamChatToolCalls(t *testing.T) {
	// Simulate a tool call response streamed across multiple chunks
	chunk1 := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}`
	chunk2 := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"","type":"","function":{"name":"","arguments":"{\"path\":"}}]},"finish_reason":null}]}`
	chunk3 := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"","type":"","function":{"name":"","arguments":"\"test.go\"}"}}]},"finish_reason":null}]}`
	chunk4 := `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`

	sseData := fmt.Sprintf("data: %s\n\ndata: %s\n\ndata: %s\n\ndata: %s\n\ndata: [DONE]\n\n",
		chunk1, chunk2, chunk3, chunk4)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", "m")
	var deltas []StreamDelta
	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, nil, func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("StreamChat error: %v", err)
	}

	// Verify tool call deltas were captured
	var allToolCalls []ToolCallDelta
	for _, d := range deltas {
		allToolCalls = append(allToolCalls, d.ToolCalls...)
	}
	if len(allToolCalls) == 0 {
		t.Fatal("no tool call deltas received")
	}

	// First chunk should have the tool call ID and function name
	if allToolCalls[0].ID != "call_123" {
		t.Errorf("first tool call ID = %q, want call_123", allToolCalls[0].ID)
	}
	if allToolCalls[0].Function.Name != "read_file" {
		t.Errorf("first tool call function name = %q, want read_file", allToolCalls[0].Function.Name)
	}
	if allToolCalls[0].Index != 0 {
		t.Errorf("first tool call index = %d, want 0", allToolCalls[0].Index)
	}

	// Accumulate arguments from all deltas
	var args strings.Builder
	for _, tc := range allToolCalls {
		args.WriteString(tc.Function.Arguments)
	}
	if args.String() != `{"path":"test.go"}` {
		t.Errorf("accumulated arguments = %q, want {\"path\":\"test.go\"}", args.String())
	}

	// Should have a Done delta from finish_reason=tool_calls
	lastNonDONE := deltas[len(deltas)-2] // second-to-last (last is the DONE)
	if !lastNonDONE.Done {
		t.Error("finish_reason=tool_calls chunk did not set Done=true")
	}
}

func TestStreamChatMultipleToolCalls(t *testing.T) {
	// Two tool calls in a single delta
	chunk := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read","arguments":"{}"}},{"index":1,"id":"call_2","type":"function","function":{"name":"write","arguments":"{}"}}]},"finish_reason":null}]}`

	sseData := fmt.Sprintf("data: %s\n\ndata: [DONE]\n\n", chunk)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", "m")
	var deltas []StreamDelta
	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, nil, func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("StreamChat error: %v", err)
	}

	// First non-Done delta should have 2 tool calls
	if len(deltas) < 1 {
		t.Fatal("no deltas received")
	}
	if len(deltas[0].ToolCalls) != 2 {
		t.Fatalf("tool calls count = %d, want 2", len(deltas[0].ToolCalls))
	}
	if deltas[0].ToolCalls[0].Index != 0 || deltas[0].ToolCalls[0].ID != "call_1" {
		t.Errorf("tool call 0: index=%d id=%q", deltas[0].ToolCalls[0].Index, deltas[0].ToolCalls[0].ID)
	}
	if deltas[0].ToolCalls[1].Index != 1 || deltas[0].ToolCalls[1].ID != "call_2" {
		t.Errorf("tool call 1: index=%d id=%q", deltas[0].ToolCalls[1].Index, deltas[0].ToolCalls[1].ID)
	}
}

// --- Error handling tests ---

func TestStreamChatServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"internal server error"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", "m")
	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, nil, func(d StreamDelta) {
		t.Error("onDelta should not be called on server error")
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "API error 500") {
		t.Errorf("error = %q, want to contain 'API error 500'", err.Error())
	}
	if !strings.Contains(err.Error(), "internal server error") {
		t.Errorf("error = %q, want to contain response body", err.Error())
	}
}

func TestStreamChatBadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad request")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", "m")
	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, nil, func(d StreamDelta) {})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "API error 400") {
		t.Errorf("error = %q, want to contain 'API error 400'", err.Error())
	}
}

func TestStreamChatUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "unauthorized")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "bad-key", "m")
	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, nil, func(d StreamDelta) {})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "API error 401") {
		t.Errorf("error = %q, want 'API error 401'", err.Error())
	}
}

func TestStreamChatConnectionRefused(t *testing.T) {
	// Use an endpoint that won't be listening
	c := NewClient("http://127.0.0.1:1", "", "m")
	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, nil, func(d StreamDelta) {})
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error = %q, want to contain 'request failed'", err.Error())
	}
}

// --- parseSSE direct tests ---

func TestParseSSEMalformedJSON(t *testing.T) {
	// Malformed JSON chunks should be silently skipped
	sseData := "data: {not valid json}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n" +
		"data: [DONE]\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	// Should have gotten the valid content delta + DONE
	contentDeltas := 0
	for _, d := range deltas {
		if d.Content == "ok" {
			contentDeltas++
		}
	}
	if contentDeltas != 1 {
		t.Errorf("expected 1 content delta with 'ok', got %d", contentDeltas)
	}
}

func TestParseSSEEmptyChoices(t *testing.T) {
	sseData := "data: {\"choices\":[]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n" +
		"data: [DONE]\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	// Empty choices chunk should be skipped; "hi" and DONE should come through
	foundHi := false
	for _, d := range deltas {
		if d.Content == "hi" {
			foundHi = true
		}
	}
	if !foundHi {
		t.Error("expected content delta 'hi' after empty choices chunk")
	}
}

func TestParseSSENonDataLines(t *testing.T) {
	// Lines without "data: " prefix should be ignored (comments, empty lines, etc.)
	sseData := ": this is a comment\n" +
		"\n" +
		"event: ping\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"yes\"},\"finish_reason\":null}]}\n\n" +
		"retry: 3000\n" +
		"data: [DONE]\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	foundYes := false
	for _, d := range deltas {
		if d.Content == "yes" {
			foundYes = true
		}
	}
	if !foundYes {
		t.Error("expected content delta 'yes'")
	}
}

func TestParseSSEDoneSignal(t *testing.T) {
	sseData := "data: [DONE]\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta (DONE), got %d", len(deltas))
	}
	if !deltas[0].Done {
		t.Error("DONE delta should have Done=true")
	}
	if deltas[0].Content != "" {
		t.Errorf("DONE delta content = %q, want empty", deltas[0].Content)
	}
}

func TestParseSSEEmptyContent(t *testing.T) {
	// Content with empty string should not set delta.Content (code checks != "")
	sseData := "data: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":null}]}\n\n" +
		"data: [DONE]\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	// First delta should have empty content (not set)
	if len(deltas) < 2 {
		t.Fatalf("expected at least 2 deltas, got %d", len(deltas))
	}
	if deltas[0].Content != "" {
		t.Errorf("empty content delta should have Content='', got %q", deltas[0].Content)
	}
}

func TestParseSSEFinishReasonWithContent(t *testing.T) {
	// A chunk can have both content and a finish_reason
	sseData := "data: {\"choices\":[{\"delta\":{\"content\":\"end\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	if len(deltas) < 1 {
		t.Fatal("expected at least 1 delta")
	}
	if deltas[0].Content != "end" {
		t.Errorf("delta content = %q, want 'end'", deltas[0].Content)
	}
	if !deltas[0].Done {
		t.Error("delta with finish_reason should have Done=true")
	}
}

func TestParseSSENoData(t *testing.T) {
	// Empty reader should produce no deltas and no error
	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(""), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}
	if len(deltas) != 0 {
		t.Errorf("expected 0 deltas from empty input, got %d", len(deltas))
	}
}

func TestParseSSENoDoneMarker(t *testing.T) {
	// Stream that ends without [DONE] -- parseSSE should still return without error
	sseData := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}
	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(deltas))
	}
	if deltas[0].Content != "partial" {
		t.Errorf("content = %q, want 'partial'", deltas[0].Content)
	}
}

// --- JSON serialization tests for types ---

func TestMessageJSONRoundTrip(t *testing.T) {
	msg := Message{
		Role:    "assistant",
		Content: "Hello",
		ToolCalls: []ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: FunctionCall{
					Name:      "bash",
					Arguments: `{"cmd":"ls"}`,
				},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Role != msg.Role {
		t.Errorf("role = %q, want %q", decoded.Role, msg.Role)
	}
	if decoded.Content != msg.Content {
		t.Errorf("content = %q, want %q", decoded.Content, msg.Content)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(decoded.ToolCalls))
	}
	if decoded.ToolCalls[0].Function.Name != "bash" {
		t.Errorf("tool call name = %q, want bash", decoded.ToolCalls[0].Function.Name)
	}
}

func TestMessageJSONOmitEmpty(t *testing.T) {
	msg := Message{Role: "user", Content: "hi"}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)

	if strings.Contains(s, "tool_calls") {
		t.Error("tool_calls should be omitted when empty")
	}
	if strings.Contains(s, "tool_call_id") {
		t.Error("tool_call_id should be omitted when empty")
	}
}

func TestToolCallMessageJSON(t *testing.T) {
	msg := Message{
		Role:       "tool",
		Content:    "file contents here",
		ToolCallID: "call_abc",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ToolCallID != "call_abc" {
		t.Errorf("tool_call_id = %q, want call_abc", decoded.ToolCallID)
	}
}

func TestChatRequestJSON(t *testing.T) {
	req := ChatRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "system", Content: "you are helpful"},
			{Role: "user", Content: "hello"},
		},
		Stream: true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)

	// Tools should be omitted
	if strings.Contains(s, "tools") {
		t.Error("tools should be omitted when nil")
	}

	var decoded ChatRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Model != "test-model" {
		t.Errorf("model = %q, want test-model", decoded.Model)
	}
	if !decoded.Stream {
		t.Error("stream should be true")
	}
	if len(decoded.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(decoded.Messages))
	}
}

func TestToolDefJSON(t *testing.T) {
	td := ToolDef{
		Type: "function",
		Function: ToolDefFunc{
			Name:        "grep",
			Description: "Search files",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"]}`),
		},
	}

	data, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ToolDef
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Function.Name != "grep" {
		t.Errorf("function name = %q, want grep", decoded.Function.Name)
	}
	if decoded.Function.Description != "Search files" {
		t.Errorf("description = %q, want 'Search files'", decoded.Function.Description)
	}
}

// --- Integration-style tests with mock server ---

func TestStreamChatFullConversation(t *testing.T) {
	// Simulate a multi-turn conversation with system, user, and assistant messages
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody ChatRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &reqBody)

		// Verify all messages came through
		if len(reqBody.Messages) != 3 {
			t.Errorf("expected 3 messages, got %d", len(reqBody.Messages))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"I can help\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "model")
	msgs := []Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "help me"},
		{Role: "assistant", Content: "sure"},
	}

	var content strings.Builder
	var doneCount int
	err := c.StreamChat(msgs, nil, func(d StreamDelta) {
		content.WriteString(d.Content)
		if d.Done {
			doneCount++
		}
	})
	if err != nil {
		t.Fatalf("StreamChat error: %v", err)
	}
	if content.String() != "I can help" {
		t.Errorf("accumulated content = %q, want 'I can help'", content.String())
	}
	if doneCount < 1 {
		t.Error("expected at least one Done signal")
	}
}

func TestStreamChatEmptyToolsOmitted(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", "m")

	// Pass empty (not nil) tools slice -- should still be omitted per len check
	err := c.StreamChat([]Message{{Role: "user", Content: "hi"}}, []ToolDef{}, func(d StreamDelta) {})
	if err != nil {
		t.Fatalf("StreamChat error: %v", err)
	}

	var reqBody ChatRequest
	json.Unmarshal(capturedBody, &reqBody)
	if reqBody.Tools != nil {
		t.Errorf("empty tools slice should not be sent, got %v", reqBody.Tools)
	}
}

func TestParseSSELargeArguments(t *testing.T) {
	// Test that large tool call arguments (up to buffer limit) work
	largeArg := strings.Repeat("x", 50000)
	chunk := fmt.Sprintf(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_big","type":"function","function":{"name":"write","arguments":"%s"}}]},"finish_reason":null}]}`, largeArg)

	sseData := fmt.Sprintf("data: %s\n\ndata: [DONE]\n\n", chunk)

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	if len(deltas) < 1 {
		t.Fatal("expected at least 1 delta")
	}
	if len(deltas[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(deltas[0].ToolCalls))
	}
	if deltas[0].ToolCalls[0].Function.Arguments != largeArg {
		t.Errorf("large argument length = %d, want %d", len(deltas[0].ToolCalls[0].Function.Arguments), len(largeArg))
	}
}

func TestParseSSEMissingFields(t *testing.T) {
	// Chunk with no content and no tool_calls -- should produce a delta with empty fields
	sseData := "data: {\"choices\":[{\"delta\":{},\"finish_reason\":null}]}\n\n" +
		"data: [DONE]\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	// First delta: empty content, no tool calls, not done
	if len(deltas) < 2 {
		t.Fatalf("expected at least 2 deltas, got %d", len(deltas))
	}
	if deltas[0].Content != "" {
		t.Errorf("content = %q, want empty", deltas[0].Content)
	}
	if len(deltas[0].ToolCalls) != 0 {
		t.Errorf("tool calls = %d, want 0", len(deltas[0].ToolCalls))
	}
	if deltas[0].Done {
		t.Error("should not be done")
	}
}

func TestParseSSEAllMalformed(t *testing.T) {
	// All chunks are malformed -- should skip everything, return no error
	sseData := "data: not-json\n\n" +
		"data: {broken\n\n" +
		"data: [DONE]\n\n"

	var deltas []StreamDelta
	err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}

	// Only the DONE delta
	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta (DONE), got %d", len(deltas))
	}
	if !deltas[0].Done {
		t.Error("expected Done=true from [DONE]")
	}
}

func TestStreamChatEndpointURLBuilding(t *testing.T) {
	// Verify URL is correctly built from endpoint with and without trailing slash
	var capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	// With trailing slash
	c := NewClient(srv.URL+"/", "", "m")
	c.StreamChat([]Message{{Role: "user", Content: "hi"}}, nil, func(d StreamDelta) {})

	if capturedPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", capturedPath)
	}
}

func TestParseSSEFinishReasons(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{"stop", "stop"},
		{"tool_calls", "tool_calls"},
		{"length", "length"},
		{"content_filter", "content_filter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sseData := fmt.Sprintf("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"%s\"}]}\n\ndata: [DONE]\n\n", tt.reason)

			var doneReceived bool
			err := parseSSE(strings.NewReader(sseData), func(d StreamDelta) {
				if d.Done && d.Content == "" && len(d.ToolCalls) == 0 {
					doneReceived = true
				}
			})
			if err != nil {
				t.Fatalf("parseSSE error: %v", err)
			}
			if !doneReceived {
				t.Errorf("finish_reason=%q did not produce Done delta", tt.reason)
			}
		})
	}
}
