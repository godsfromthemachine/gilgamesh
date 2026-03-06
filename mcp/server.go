package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/godsfromthemachine/gilgamesh/hooks"
	"github.com/godsfromthemachine/gilgamesh/session"
	"github.com/godsfromthemachine/gilgamesh/tools"
)

// Server implements the MCP protocol over stdio (JSON-RPC 2.0).
type Server struct {
	registry *tools.Registry
	hooks    *hooks.Registry
	session  *session.Logger
	version  string
}

func NewServer(registry *tools.Registry, hookReg *hooks.Registry, sessLog *session.Logger, version string) *Server {
	return &Server{
		registry: registry,
		hooks:    hookReg,
		session:  sessLog,
		version:  version,
	}
}

// Run starts the MCP server reading from stdin and writing to stdout.
func (s *Server) Run() error {
	fmt.Fprintf(os.Stderr, "gilgamesh MCP server v%s ready\n", s.version)
	return s.Serve(os.Stdin, os.Stdout)
}

// Serve reads JSON-RPC requests from r and writes responses to w.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	enc := json.NewEncoder(w)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			enc.Encode(Response{
				JSONRPC: "2.0",
				Error:   &RPCError{Code: ParseError, Message: "parse error"},
			})
			continue
		}

		resp := s.handleRequest(req)
		if resp != nil {
			enc.Encode(resp)
		}
	}
	return scanner.Err()
}

func (s *Server) handleRequest(req Request) *Response {
	// Notifications (no ID) like "notifications/initialized" get no response
	if req.ID == nil || string(req.ID) == "null" {
		return nil
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: MethodNotFound, Message: "method not found: " + req.Method},
		}
	}
}

func (s *Server) handleInitialize(req Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo: ServerInfo{
				Name:    "gilgamesh",
				Version: s.version,
			},
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{},
			},
		},
	}
}

func (s *Server) handleToolsList(req Request) *Response {
	toolMap := s.registry.Tools()
	mcpTools := make([]MCPTool, 0, len(toolMap))
	for _, t := range toolMap {
		mcpTools = append(mcpTools, MCPTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ToolsListResult{Tools: mcpTools},
	}
}

func (s *Server) handleToolsCall(req Request) *Response {
	var params ToolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: InvalidParams, Message: err.Error()},
		}
	}

	// Run pre-hooks
	if s.hooks.HasHooks() {
		preResults := s.hooks.Run(hooks.PreHook, params.Name, params.Arguments, "")
		for _, r := range preResults {
			if r.Err != nil {
				return &Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: ToolsCallResult{
						Content: []ToolContent{{Type: "text", Text: "Blocked by pre-hook: " + r.Err.Error()}},
						IsError: true,
					},
				}
			}
		}
	}

	s.session.Log(session.Entry{Type: "tool_call", Tool: params.Name, Args: params.Arguments})

	start := time.Now()
	result, err := s.registry.Execute(params.Name, params.Arguments)
	elapsed := time.Since(start)

	s.session.Log(session.Entry{
		Type:     "tool_result",
		Tool:     params.Name,
		Content:  truncate(result, 500),
		Duration: elapsed,
	})

	// Run post-hooks
	if s.hooks.HasHooks() {
		s.hooks.Run(hooks.PostHook, params.Name, params.Arguments, result)
	}

	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolsCallResult{
				Content: []ToolContent{{Type: "text", Text: "Error: " + err.Error()}},
				IsError: true,
			},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ToolsCallResult{
			Content: []ToolContent{{Type: "text", Text: result}},
		},
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
