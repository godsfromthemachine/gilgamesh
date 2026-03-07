package ui

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyNetworkError(t *testing.T) {
	tests := []struct {
		msg  string
		want Category
	}{
		{"request failed: dial tcp 127.0.0.1:8081: connection refused", CatNetwork},
		{"connection refused", CatNetwork},
		{"no such host", CatNetwork},
		{"API error 401: Unauthorized", CatAuth},
		{"API error 403: forbidden", CatAuth},
		{"context deadline exceeded", CatTimeout},
		{"timeout waiting for response", CatTimeout},
		{"invalid JSON in response", CatLLM},
		{"json: cannot unmarshal string", CatLLM},
		{"something else", CatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			ge := ClassifyError(errors.New(tt.msg))
			if ge.Cat != tt.want {
				t.Errorf("ClassifyError(%q).Cat = %d, want %d", tt.msg, ge.Cat, tt.want)
			}
		})
	}
}

func TestClassifyNil(t *testing.T) {
	if ClassifyError(nil) != nil {
		t.Error("ClassifyError(nil) should return nil")
	}
}

func TestClassifyPreserveGilgaError(t *testing.T) {
	ge := &GilgaError{Cat: CatConfig, Inner: errors.New("bad config"), Hint: "fix it"}
	got := ClassifyError(ge)
	if got != ge {
		t.Error("ClassifyError should return existing GilgaError as-is")
	}
}

func TestGilgaErrorUnwrap(t *testing.T) {
	inner := errors.New("original")
	ge := &GilgaError{Cat: CatNetwork, Inner: inner}
	if !errors.Is(ge, inner) {
		t.Error("Unwrap should allow errors.Is to find inner error")
	}
}

func TestFormatErrorWithHint(t *testing.T) {
	profile = NoColor
	err := errors.New("dial tcp 127.0.0.1:8081: connection refused")
	got := FormatError(err)
	if !strings.Contains(got, "error:") {
		t.Error("missing 'error:' prefix")
	}
	if !strings.Contains(got, "hint:") {
		t.Error("missing 'hint:' line")
	}
}

func TestFormatErrorNoHint(t *testing.T) {
	profile = NoColor
	err := errors.New("something unexpected")
	got := FormatError(err)
	if strings.Contains(got, "hint:") {
		t.Error("unknown error should not have hint")
	}
}

func TestFormatErrorNil(t *testing.T) {
	if FormatError(nil) != "" {
		t.Error("FormatError(nil) should return empty string")
	}
}
