package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/godsfromthemachine/gilgamesh/hooks"
	"github.com/godsfromthemachine/gilgamesh/session"
	"github.com/godsfromthemachine/gilgamesh/tools"
)

func newTestServer() *Server {
	return NewServer(
		tools.NewRegistry(),
		&hooks.Registry{},
		&session.Logger{},
		"test",
	)
}

func rpc(method string, id int, params string) string {
	if params == "" {
		return `{"jsonrpc":"2.0","id":` + itoa(id) + `,"method":"` + method + `"}`
	}
	return `{"jsonrpc":"2.0","id":` + itoa(id) + `,"method":"` + method + `","params":` + params + `}`
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

func TestInitialize(t *testing.T) {
	srv := newTestServer()
	input := rpc("initialize", 1, `{"protocolVersion":"2024-11-05","clientInfo":{"name":"test","version":"1.0"},"capabilities":{}}`)
	var out bytes.Buffer
	srv.Serve(strings.NewReader(input+"\n"), &out)

	var resp Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, _ := json.Marshal(resp.Result)
	var init InitializeResult
	json.Unmarshal(result, &init)

	if init.ServerInfo.Name != "gilgamesh" {
		t.Errorf("server name = %q, want gilgamesh", init.ServerInfo.Name)
	}
	if init.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocol = %q, want 2024-11-05", init.ProtocolVersion)
	}
	if init.Capabilities.Tools == nil {
		t.Error("tools capability is nil")
	}
}

func TestToolsList(t *testing.T) {
	srv := newTestServer()
	input := rpc("tools/list", 1, "")
	var out bytes.Buffer
	srv.Serve(strings.NewReader(input+"\n"), &out)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, _ := json.Marshal(resp.Result)
	var list ToolsListResult
	json.Unmarshal(result, &list)

	if len(list.Tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(list.Tools))
	}

	names := make(map[string]bool)
	for _, tool := range list.Tools {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
		if len(tool.InputSchema) == 0 {
			t.Errorf("tool %s has empty inputSchema", tool.Name)
		}
	}

	for _, expected := range []string{"read", "write", "edit", "bash", "grep", "glob", "test"} {
		if !names[expected] {
			t.Errorf("missing tool: %s", expected)
		}
	}
}

func TestToolsCall(t *testing.T) {
	srv := newTestServer()
	input := rpc("tools/call", 1, `{"name":"glob","arguments":{"pattern":"*.go"}}`)
	var out bytes.Buffer
	srv.Serve(strings.NewReader(input+"\n"), &out)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, _ := json.Marshal(resp.Result)
	var callResult ToolsCallResult
	json.Unmarshal(result, &callResult)

	if callResult.IsError {
		t.Fatal("tool call returned error")
	}
	if len(callResult.Content) == 0 {
		t.Fatal("empty content")
	}
	if callResult.Content[0].Type != "text" {
		t.Errorf("content type = %q, want text", callResult.Content[0].Type)
	}
	if !strings.Contains(callResult.Content[0].Text, "server.go") {
		t.Error("glob result missing server.go")
	}
}

func TestToolsCallUnknown(t *testing.T) {
	srv := newTestServer()
	input := rpc("tools/call", 1, `{"name":"nonexistent","arguments":{}}`)
	var out bytes.Buffer
	srv.Serve(strings.NewReader(input+"\n"), &out)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)

	result, _ := json.Marshal(resp.Result)
	var callResult ToolsCallResult
	json.Unmarshal(result, &callResult)

	if !callResult.IsError {
		t.Fatal("expected isError=true for unknown tool")
	}
}

func TestUnknownMethod(t *testing.T) {
	srv := newTestServer()
	input := rpc("nonexistent", 1, "")
	var out bytes.Buffer
	srv.Serve(strings.NewReader(input+"\n"), &out)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != MethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, MethodNotFound)
	}
}

func TestNotificationNoResponse(t *testing.T) {
	srv := newTestServer()
	// Notification has no id
	input := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	var out bytes.Buffer
	srv.Serve(strings.NewReader(input+"\n"), &out)

	if out.Len() > 0 {
		t.Errorf("notification should produce no response, got: %s", out.String())
	}
}

func TestMultipleRequests(t *testing.T) {
	srv := newTestServer()
	input := strings.Join([]string{
		rpc("initialize", 1, `{"protocolVersion":"2024-11-05","clientInfo":{"name":"test","version":"1.0"},"capabilities":{}}`),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		rpc("tools/list", 2, ""),
	}, "\n") + "\n"

	var out bytes.Buffer
	srv.Serve(strings.NewReader(input), &out)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// Should get 2 responses (initialize + tools/list, notification is silent)
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d: %v", len(lines), lines)
	}
}
