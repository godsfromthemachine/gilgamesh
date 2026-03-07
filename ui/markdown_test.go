package ui

import (
	"strings"
	"testing"
)

func TestStreamRendererPlainText(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("hello world\n")
	if out != "hello world\n" {
		t.Errorf("plain text = %q, want %q", out, "hello world\n")
	}
}

func TestStreamRendererCodeBlock(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	var buf strings.Builder
	tokens := []string{"```go\n", "func main() {\n", "}\n", "```\n"}
	for _, tok := range tokens {
		buf.WriteString(r.WriteToken(tok))
	}
	buf.WriteString(r.Flush())

	result := buf.String()

	// Should contain the language label
	if !strings.Contains(result, "go") {
		t.Error("missing language label")
	}
	// Should contain the border character
	if !strings.Contains(result, "\u2502") {
		t.Error("missing code block border")
	}
	// Should contain "func" (highlighted as bold)
	if !strings.Contains(result, "func") {
		t.Error("missing 'func' keyword")
	}
}

func TestStreamRendererNoColor(t *testing.T) {
	profile = NoColor
	r := NewStreamRenderer()

	input := "```go\nfunc main() {}\n```\n"
	out := r.WriteToken(input)
	out += r.Flush()

	// With NoColor, output should be identical to input (passthrough)
	if out != input {
		t.Errorf("NoColor output = %q, want %q", out, input)
	}
}

func TestStreamRendererPartialTokens(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	var buf strings.Builder
	// Feed character by character
	input := "hello\n"
	for _, ch := range input {
		buf.WriteString(r.WriteToken(string(ch)))
	}
	buf.WriteString(r.Flush())

	if buf.String() != "hello\n" {
		t.Errorf("partial token result = %q, want %q", buf.String(), "hello\n")
	}
}

func TestStreamRendererFlushPartial(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	// Write without newline
	out := r.WriteToken("partial")
	if out != "" {
		t.Errorf("WriteToken without newline should return empty, got %q", out)
	}

	flushed := r.Flush()
	if flushed != "partial" {
		t.Errorf("Flush() = %q, want %q", flushed, "partial")
	}
}

func TestHighlightKeywords(t *testing.T) {
	profile = ANSI16

	line := "func main() {"
	result := highlightKeywords(line, keywords["go"])

	if !strings.Contains(result, "\033[1m") {
		t.Error("expected bold ANSI codes for keyword 'func'")
	}
	if !strings.Contains(result, "main") {
		t.Error("missing 'main' in output")
	}
}

func TestHighlightStrings(t *testing.T) {
	profile = ANSI16

	line := `fmt.Println("hello")`
	result := highlightKeywords(line, keywords["go"])

	// Should contain green color for string
	if !strings.Contains(result, "\033[32m") {
		t.Error("expected green ANSI for string literal")
	}
}

func TestHighlightComments(t *testing.T) {
	profile = ANSI16

	got := highlightLine("  // this is a comment", "go")
	if !strings.Contains(got, "\033[2m") {
		t.Error("expected dim ANSI for comment")
	}

	got = highlightLine("  # python comment", "python")
	if !strings.Contains(got, "\033[2m") {
		t.Error("expected dim ANSI for python comment")
	}
}

func TestStreamRendererHeaders(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("# Hello World\n")
	if !strings.Contains(out, "\033[1m") {
		t.Error("H1 should be bold")
	}
	if !strings.Contains(out, "Hello World") {
		t.Error("missing header text")
	}

	out = r.WriteToken("## Subtitle\n")
	if !strings.Contains(out, "Subtitle") {
		t.Error("missing H2 text")
	}
}

func TestStreamRendererBoldInline(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("This is **bold** text\n")
	if !strings.Contains(out, "\033[1m") {
		t.Error("expected bold ANSI for **bold**")
	}
	if !strings.Contains(out, "bold") {
		t.Error("missing bold text")
	}
}

func TestStreamRendererItalicInline(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("This is *italic* text\n")
	if !strings.Contains(out, "\033[2m") {
		t.Error("expected dim ANSI for *italic*")
	}
}

func TestStreamRendererInlineCode(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("Use `go build` here\n")
	if !strings.Contains(out, "\033[90m") {
		t.Error("expected muted ANSI for `inline code`")
	}
}

func TestStreamRendererUnorderedList(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("- item one\n")
	if !strings.Contains(out, "•") {
		t.Error("expected bullet char for unordered list")
	}
	if !strings.Contains(out, "item one") {
		t.Error("missing list item text")
	}
}

func TestStreamRendererOrderedList(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("1. first\n")
	if !strings.Contains(out, "1.") {
		t.Error("missing ordered list number")
	}
	if !strings.Contains(out, "first") {
		t.Error("missing ordered list text")
	}
}

func TestStreamRendererBlockquote(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("> quote text\n")
	if !strings.Contains(out, ">") {
		t.Error("missing blockquote marker")
	}
	if !strings.Contains(out, "quote text") {
		t.Error("missing blockquote text")
	}
}

func TestStreamRendererHorizontalRule(t *testing.T) {
	profile = ANSI16
	r := NewStreamRenderer()

	out := r.WriteToken("---\n")
	if !strings.Contains(out, "─") {
		t.Error("expected horizontal rule chars")
	}
}

func TestStreamRendererNoColorPassthrough(t *testing.T) {
	profile = NoColor
	r := NewStreamRenderer()

	input := "# Header\n**bold** and *italic*\n- list\n> quote\n---\n"
	out := r.WriteToken(input)
	out += r.Flush()

	// NoColor mode should pass through without formatting
	if out != input {
		t.Errorf("NoColor output = %q, want %q", out, input)
	}
}

func TestRenderInlineNoColor(t *testing.T) {
	profile = NoColor
	got := renderInline("**bold** and *italic*")
	if got != "**bold** and *italic*" {
		t.Errorf("renderInline NoColor = %q, want passthrough", got)
	}
}

func TestLangAliases(t *testing.T) {
	tests := []struct {
		alias, resolved string
	}{
		{"golang", "go"},
		{"javascript", "js"},
		{"typescript", "ts"},
		{"py", "python"},
		{"rs", "rust"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			got, ok := langAliases[tt.alias]
			if !ok {
				t.Fatalf("alias %q not found", tt.alias)
			}
			if got != tt.resolved {
				t.Errorf("alias %q = %q, want %q", tt.alias, got, tt.resolved)
			}
		})
	}
}
