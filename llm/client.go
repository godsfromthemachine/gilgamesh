package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDef struct {
	Type     string      `json:"type"`
	Function ToolDefFunc `json:"function"`
}

type ToolDefFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []ToolDef `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
}

// ToolCallDelta is a streaming chunk of a tool call, with its index.
type ToolCallDelta struct {
	Index int
	ToolCall
}

// StreamDelta represents a single SSE chunk from the streaming API.
type StreamDelta struct {
	Content   string          // text content delta
	ToolCalls []ToolCallDelta // tool call deltas with index
	Done      bool            // true when finish_reason is set
}

type Client struct {
	endpoint   string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewClient(endpoint, apiKey, model string) *Client {
	return &Client{
		endpoint:   strings.TrimSuffix(endpoint, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// StreamChat sends a streaming chat completion request and calls onDelta for each chunk.
func (c *Client) StreamChat(messages []Message, tools []ToolDef, onDelta func(StreamDelta)) error {
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	}
	if len(tools) > 0 {
		req.Tools = tools
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return parseSSE(resp.Body, onDelta)
}

func parseSSE(r io.Reader, onDelta func(StreamDelta)) error {
	scanner := bufio.NewScanner(r)
	// Increase buffer for large tool call arguments
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			onDelta(StreamDelta{Done: true})
			return nil
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := StreamDelta{}

		if choice.Delta.Content != "" {
			delta.Content = choice.Delta.Content
		}

		for _, tc := range choice.Delta.ToolCalls {
			delta.ToolCalls = append(delta.ToolCalls, ToolCallDelta{
				Index: tc.Index,
				ToolCall: ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				},
			})
		}

		if choice.FinishReason != nil {
			delta.Done = true
		}

		onDelta(delta)
	}

	return scanner.Err()
}
