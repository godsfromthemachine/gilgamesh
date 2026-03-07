package ui

import (
	"fmt"
	"strings"
)

// Category classifies error types for contextual hints.
type Category int

const (
	CatUnknown Category = iota
	CatNetwork
	CatAuth
	CatTimeout
	CatConfig
	CatTool
	CatLLM
)

// GilgaError wraps an error with a category and recovery hint.
type GilgaError struct {
	Cat   Category
	Inner error
	Hint  string
}

func (e *GilgaError) Error() string {
	return e.Inner.Error()
}

func (e *GilgaError) Unwrap() error {
	return e.Inner
}

// ClassifyError inspects an error's message and returns a GilgaError with a hint.
// If the error is already a *GilgaError, it is returned as-is.
// Unknown errors get no hint.
func ClassifyError(err error) *GilgaError {
	if err == nil {
		return nil
	}
	if ge, ok := err.(*GilgaError); ok {
		return ge
	}

	msg := err.Error()
	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "dial tcp"):
		return &GilgaError{
			Cat:   CatNetwork,
			Inner: err,
			Hint:  "Is the LLM server running? Check the endpoint in gilgamesh.json",
		}
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "dns"):
		return &GilgaError{
			Cat:   CatNetwork,
			Inner: err,
			Hint:  "Cannot resolve hostname. Check the endpoint URL",
		}
	case strings.Contains(msg, "401") || strings.Contains(lower, "unauthorized"):
		return &GilgaError{
			Cat:   CatAuth,
			Inner: err,
			Hint:  "Check api_key in gilgamesh.json",
		}
	case strings.Contains(msg, "403") || strings.Contains(lower, "forbidden"):
		return &GilgaError{
			Cat:   CatAuth,
			Inner: err,
			Hint:  "API key lacks permission. Check api_key in gilgamesh.json",
		}
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return &GilgaError{
			Cat:   CatTimeout,
			Inner: err,
			Hint:  "Request timed out. The model may be overloaded or ctx-size too large",
		}
	case strings.Contains(lower, "invalid json") || strings.Contains(lower, "unmarshal") || strings.Contains(lower, "unexpected end of json"):
		return &GilgaError{
			Cat:   CatLLM,
			Inner: err,
			Hint:  "Malformed response from LLM. The model may not support tool calling",
		}
	default:
		return &GilgaError{
			Cat:   CatUnknown,
			Inner: err,
		}
	}
}

// FormatError classifies an error and formats it with a hint line if available.
func FormatError(err error) string {
	ge := ClassifyError(err)
	if ge == nil {
		return ""
	}

	msg := ToolError(fmt.Sprintf("error: %s", ge.Inner))
	if ge.Hint != "" {
		msg += "\n" + Muted(fmt.Sprintf("  hint: %s", ge.Hint))
	}
	return msg
}
