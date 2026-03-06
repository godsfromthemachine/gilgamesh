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

// Execute runs a tool by name with the given arguments.
func (r *Registry) Execute(name string, args json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(args)
}

// schema is a helper to create JSON Schema bytes inline.
func schema(s string) json.RawMessage {
	return json.RawMessage(s)
}
