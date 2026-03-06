package tools

import (
	"encoding/json"
	"fmt"

	"github.com/godsfromthemachine/gilgamesh/llm"
)

// Tool defines a callable tool with its schema and handler.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema for parameters
	Execute     func(args json.RawMessage) (string, error)
}

// Registry holds all available tools.
type Registry struct {
	tools map[string]*Tool
}

func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]*Tool)}
	r.Register(ReadTool())
	r.Register(WriteTool())
	r.Register(EditTool())
	r.Register(BashTool())
	r.Register(GrepTool())
	r.Register(GlobTool())
	r.Register(TestTool())
	return r
}

func (r *Registry) Register(t *Tool) {
	r.tools[t.Name] = t
}

// Definitions returns the tool definitions for the LLM API.
func (r *Registry) Definitions() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.ToolDefFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return defs
}

// Tools returns the raw tool map for external protocol adapters (MCP, HTTP).
func (r *Registry) Tools() map[string]*Tool {
	return r.tools
}

// Execute runs a tool by name with the given arguments.
func (r *Registry) Execute(name string, args json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(args)
}

// Filter restricts the registry to only allowed tools and removes denied tools.
// If allowed is non-empty, only tools in the list are kept.
// Denied tools are always removed regardless of the allowed list.
func (r *Registry) Filter(allowed, denied []string) {
	if len(allowed) > 0 {
		keep := make(map[string]bool, len(allowed))
		for _, name := range allowed {
			keep[name] = true
		}
		for name := range r.tools {
			if !keep[name] {
				delete(r.tools, name)
			}
		}
	}
	for _, name := range denied {
		delete(r.tools, name)
	}
}

// schema is a helper to create JSON Schema bytes inline.
func schema(s string) json.RawMessage {
	return json.RawMessage(s)
}
