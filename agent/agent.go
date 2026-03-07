package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gilgacontext "github.com/godsfromthemachine/gilgamesh/context"
	"github.com/godsfromthemachine/gilgamesh/hooks"
	"github.com/godsfromthemachine/gilgamesh/llm"
	"github.com/godsfromthemachine/gilgamesh/memory"
	"github.com/godsfromthemachine/gilgamesh/session"
	"github.com/godsfromthemachine/gilgamesh/tools"
)

const maxToolLoops = 15

// compactThreshold is the estimated token count at which we start compacting old tool results.
const compactThreshold = 12000

type Agent struct {
	client   *llm.Client
	registry *tools.Registry
	history  []llm.Message
	hooks    *hooks.Registry
	session  *session.Logger
	memory   *memory.Store
}

func New(client *llm.Client, hookReg *hooks.Registry, sessLog *session.Logger, mem *memory.Store) *Agent {
	return &Agent{
		client:   client,
		registry: tools.NewRegistry(),
		history: []llm.Message{
			{Role: "system", Content: buildSystemPrompt(mem)},
		},
		hooks:   hookReg,
		session: sessLog,
		memory:  mem,
	}
}

// buildSystemPrompt combines the base prompt with project context and memory.
func buildSystemPrompt(mem *memory.Store) string {
	base := SystemPrompt()

	projectCtx := gilgacontext.Load()
	if projectCtx != "" {
		tokens := gilgacontext.TokenEstimate(projectCtx)
		if tokens > 500 {
			runes := []rune(projectCtx)
			if len(runes) > 2000 {
				projectCtx = string(runes[:2000]) + "\n...(truncated)"
			}
		}
		base += "\n\nProject context:\n" + projectCtx
	}

	if mem != nil {
		memPrompt := mem.FormatForPrompt()
		if memPrompt != "" {
			base += "\n\n" + memPrompt
		}
	}

	return base
}

func (a *Agent) ClearHistory() {
	a.history = []llm.Message{
		{Role: "system", Content: buildSystemPrompt(a.memory)},
	}
}

func (a *Agent) SetClient(client *llm.Client) {
	a.client = client
}

func (a *Agent) EstimateTokens() int {
	total := 0
	for _, m := range a.history {
		total += len(m.Content) / 4
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments) / 4
			total += len(tc.Function.Name) / 4
		}
	}
	return total
}

// Run sends a user message and processes the full agent loop (including tool calls).
func (a *Agent) Run(userMessage string) error {
	a.history = append(a.history, llm.Message{Role: "user", Content: userMessage})
	a.session.Log(session.Entry{Type: "user", Content: userMessage})

	// Track recent tool calls to detect loops
	recentCalls := make(map[string]int) // "tool:args" -> count

	for loop := 0; loop < maxToolLoops; loop++ {
		// Compact context if approaching threshold
		a.maybeCompact()

		content, toolCalls, err := a.streamResponse()
		if err != nil {
			return err
		}

		// Append assistant message to history
		assistantMsg := llm.Message{Role: "assistant"}
		if content != "" {
			assistantMsg.Content = content
			a.session.Log(session.Entry{Type: "assistant", Content: content})
		}
		if len(toolCalls) > 0 {
			assistantMsg.ToolCalls = toolCalls
		}
		a.history = append(a.history, assistantMsg)

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			return nil
		}

		// Check for repeated tool calls (loop detection)
		loopDetected := false
		for _, tc := range toolCalls {
			key := tc.Function.Name + ":" + tc.Function.Arguments
			recentCalls[key]++
			if recentCalls[key] >= 2 {
				loopDetected = true
			}
		}
		if loopDetected {
			fmt.Printf("\n\033[33m[loop detected — forcing response]\033[0m\n")
			// Inject a message telling the model to stop and respond
			a.history = append(a.history, llm.Message{
				Role:    "user",
				Content: "You are repeating tool calls. Stop using tools and provide your response now based on the information you already have.",
			})
			// One more attempt without getting stuck
			content, _, err := a.streamResponse()
			if err != nil {
				return err
			}
			if content != "" {
				a.history = append(a.history, llm.Message{Role: "assistant", Content: content})
				a.session.Log(session.Entry{Type: "assistant", Content: content})
			}
			return nil
		}

		// Execute tool calls and append results
		for _, tc := range toolCalls {
			fmt.Printf("\n\033[36m⚡ %s\033[0m", tc.Function.Name)
			args := json.RawMessage(tc.Function.Arguments)

			// Show brief args
			var argsMap map[string]interface{}
			if json.Unmarshal(args, &argsMap) == nil {
				if cmd, ok := argsMap["command"]; ok {
					fmt.Printf(" → %s", truncate(fmt.Sprint(cmd), 80))
				} else if path, ok := argsMap["path"]; ok {
					fmt.Printf(" → %s", path)
				} else if pattern, ok := argsMap["pattern"]; ok {
					fmt.Printf(" → %s", pattern)
				}
			}
			fmt.Println()

			// Run pre-hooks
			if a.hooks.HasHooks() {
				preResults := a.hooks.Run(hooks.PreHook, tc.Function.Name, args, "")
				for _, r := range preResults {
					if r.Err != nil {
						fmt.Printf("\033[33m  hook blocked: %s\033[0m\n", r.Err)
						a.history = append(a.history, llm.Message{
							Role:       "tool",
							Content:    fmt.Sprintf("Blocked by pre-hook: %s", r.Err),
							ToolCallID: tc.ID,
						})
						continue
					}
				}
			}

			start := time.Now()
			result, err := a.registry.Execute(tc.Function.Name, args)
			elapsed := time.Since(start)

			a.session.Log(session.Entry{
				Type:     "tool_call",
				Tool:     tc.Function.Name,
				Args:     args,
				Duration: elapsed,
			})

			if err != nil {
				result = fmt.Sprintf("Error: %s", err)
				fmt.Printf("\033[31m  ✗ %s (%s)\033[0m\n", err, elapsed.Round(time.Millisecond))
			} else {
				lines := strings.Count(result, "\n")
				fmt.Printf("\033[32m  ✓ %d lines (%s)\033[0m\n", lines+1, elapsed.Round(time.Millisecond))
			}

			// Run post-hooks
			if a.hooks.HasHooks() {
				a.hooks.Run(hooks.PostHook, tc.Function.Name, args, result)
			}

			a.session.Log(session.Entry{Type: "tool_result", Tool: tc.Function.Name, Content: truncate(result, 500)})

			a.history = append(a.history, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return fmt.Errorf("tool loop limit (%d) reached", maxToolLoops)
}

// maybeCompact reduces context size by trimming old tool results when approaching the threshold.
func (a *Agent) maybeCompact() {
	tokens := a.EstimateTokens()
	if tokens < compactThreshold {
		return
	}

	// Compact strategy: truncate large tool results from older messages.
	// Keep the system prompt (index 0) and the last 4 messages intact.
	keepTail := 4
	if len(a.history) <= keepTail+1 {
		return
	}

	compacted := 0
	for i := 1; i < len(a.history)-keepTail; i++ {
		m := &a.history[i]
		if m.Role == "tool" && len(m.Content) > 200 {
			// Truncate to a summary
			lines := strings.SplitN(m.Content, "\n", 4)
			if len(lines) > 3 {
				m.Content = strings.Join(lines[:3], "\n") + "\n...(compacted)"
			}
			compacted++
		}
	}

	if compacted > 0 {
		newTokens := a.EstimateTokens()
		fmt.Printf("\033[90m[compacted %d tool results: ~%d → ~%d tokens]\033[0m\n", compacted, tokens, newTokens)
	}
}

// streamResponse streams LLM output to terminal and collects tool calls.
func (a *Agent) streamResponse() (string, []llm.ToolCall, error) {
	var contentBuf strings.Builder
	toolCallMap := make(map[int]*llm.ToolCall)
	firstToken := true

	err := a.client.StreamChat(a.history, a.registry.Definitions(), func(delta llm.StreamDelta) {
		if delta.Content != "" {
			if firstToken {
				fmt.Print("\n")
				firstToken = false
			}
			fmt.Print(delta.Content)
			contentBuf.WriteString(delta.Content)
		}

		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if existing, ok := toolCallMap[idx]; ok {
				existing.Function.Arguments += tc.Function.Arguments
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				if tc.ID != "" {
					existing.ID = tc.ID
				}
			} else {
				toolCallMap[idx] = &llm.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	})

	if contentBuf.Len() > 0 {
		fmt.Println()
	}

	if err != nil {
		return "", nil, err
	}

	var toolCalls []llm.ToolCall
	for _, tc := range toolCallMap {
		toolCalls = append(toolCalls, *tc)
	}

	return contentBuf.String(), toolCalls, nil
}

// EventType identifies agent events for non-terminal consumers (HTTP API).
type EventType string

const (
	EventContent    EventType = "content"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventError      EventType = "error"
	EventDone       EventType = "done"
)

// Event is emitted during agent execution for programmatic consumers.
type Event struct {
	Type    EventType       `json:"type"`
	Content string          `json:"content,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Args    json.RawMessage `json:"args,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// RunWithEvents is like Run but emits structured events instead of printing to terminal.
func (a *Agent) RunWithEvents(userMessage string, emit func(Event)) error {
	a.history = append(a.history, llm.Message{Role: "user", Content: userMessage})
	a.session.Log(session.Entry{Type: "user", Content: userMessage})

	recentCalls := make(map[string]int)

	for loop := 0; loop < maxToolLoops; loop++ {
		a.maybeCompactQuiet()

		content, toolCalls, err := a.streamResponseWithEvents(emit)
		if err != nil {
			return err
		}

		assistantMsg := llm.Message{Role: "assistant"}
		if content != "" {
			assistantMsg.Content = content
			a.session.Log(session.Entry{Type: "assistant", Content: content})
		}
		if len(toolCalls) > 0 {
			assistantMsg.ToolCalls = toolCalls
		}
		a.history = append(a.history, assistantMsg)

		if len(toolCalls) == 0 {
			return nil
		}

		// Loop detection
		loopDetected := false
		for _, tc := range toolCalls {
			key := tc.Function.Name + ":" + tc.Function.Arguments
			recentCalls[key]++
			if recentCalls[key] >= 2 {
				loopDetected = true
			}
		}
		if loopDetected {
			emit(Event{Type: EventError, Error: "loop detected — forcing response"})
			a.history = append(a.history, llm.Message{
				Role:    "user",
				Content: "You are repeating tool calls. Stop using tools and provide your response now based on the information you already have.",
			})
			content, _, err := a.streamResponseWithEvents(emit)
			if err != nil {
				return err
			}
			if content != "" {
				a.history = append(a.history, llm.Message{Role: "assistant", Content: content})
				a.session.Log(session.Entry{Type: "assistant", Content: content})
			}
			return nil
		}

		// Execute tool calls
		for _, tc := range toolCalls {
			args := json.RawMessage(tc.Function.Arguments)
			emit(Event{Type: EventToolCall, Tool: tc.Function.Name, Args: args})

			// Pre-hooks
			if a.hooks.HasHooks() {
				preResults := a.hooks.Run(hooks.PreHook, tc.Function.Name, args, "")
				for _, r := range preResults {
					if r.Err != nil {
						emit(Event{Type: EventError, Error: "hook blocked: " + r.Err.Error()})
						a.history = append(a.history, llm.Message{
							Role:       "tool",
							Content:    fmt.Sprintf("Blocked by pre-hook: %s", r.Err),
							ToolCallID: tc.ID,
						})
						continue
					}
				}
			}

			start := time.Now()
			result, execErr := a.registry.Execute(tc.Function.Name, args)
			elapsed := time.Since(start)

			a.session.Log(session.Entry{
				Type:     "tool_call",
				Tool:     tc.Function.Name,
				Args:     args,
				Duration: elapsed,
			})

			if execErr != nil {
				result = fmt.Sprintf("Error: %s", execErr)
			}

			// Post-hooks
			if a.hooks.HasHooks() {
				a.hooks.Run(hooks.PostHook, tc.Function.Name, args, result)
			}

			a.session.Log(session.Entry{Type: "tool_result", Tool: tc.Function.Name, Content: truncate(result, 500)})

			emit(Event{Type: EventToolResult, Tool: tc.Function.Name, Content: result})

			a.history = append(a.history, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return fmt.Errorf("tool loop limit (%d) reached", maxToolLoops)
}

// streamResponseWithEvents streams LLM output via events instead of terminal.
func (a *Agent) streamResponseWithEvents(emit func(Event)) (string, []llm.ToolCall, error) {
	var contentBuf strings.Builder
	toolCallMap := make(map[int]*llm.ToolCall)

	err := a.client.StreamChat(a.history, a.registry.Definitions(), func(delta llm.StreamDelta) {
		if delta.Content != "" {
			emit(Event{Type: EventContent, Content: delta.Content})
			contentBuf.WriteString(delta.Content)
		}

		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if existing, ok := toolCallMap[idx]; ok {
				existing.Function.Arguments += tc.Function.Arguments
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				if tc.ID != "" {
					existing.ID = tc.ID
				}
			} else {
				toolCallMap[idx] = &llm.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	})

	if err != nil {
		return "", nil, err
	}

	var toolCalls []llm.ToolCall
	for _, tc := range toolCallMap {
		toolCalls = append(toolCalls, *tc)
	}

	return contentBuf.String(), toolCalls, nil
}

// maybeCompactQuiet is like maybeCompact but without terminal output.
func (a *Agent) maybeCompactQuiet() {
	tokens := a.EstimateTokens()
	if tokens < compactThreshold {
		return
	}

	keepTail := 4
	if len(a.history) <= keepTail+1 {
		return
	}

	for i := 1; i < len(a.history)-keepTail; i++ {
		m := &a.history[i]
		if m.Role == "tool" && len(m.Content) > 200 {
			lines := strings.SplitN(m.Content, "\n", 4)
			if len(lines) > 3 {
				m.Content = strings.Join(lines[:3], "\n") + "\n...(compacted)"
			}
		}
	}
}

// Registry returns the agent's tool registry for external consumers.
func (a *Agent) Registry() *tools.Registry {
	return a.registry
}

// History returns the current conversation history.
func (a *Agent) History() []llm.Message {
	return a.history
}

// LoadHistory replaces the agent's conversation history (e.g., to resume a session).
// The first message must be a system prompt; if not, it prepends one.
func (a *Agent) LoadHistory(history []llm.Message) {
	if len(history) == 0 {
		return
	}
	if history[0].Role != "system" {
		a.history = append([]llm.Message{{Role: "system", Content: buildSystemPrompt(a.memory)}}, history...)
	} else {
		// Replace system prompt with current one (context/memory may have changed)
		history[0].Content = buildSystemPrompt(a.memory)
		a.history = history
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
