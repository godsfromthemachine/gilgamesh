package ui

import "testing"

func TestStylesWithColor(t *testing.T) {
	profile = ANSI16

	tests := []struct {
		name string
		fn   func(string) string
		in   string
		want string
	}{
		{"Bold", Bold, "hi", "\033[1mhi\033[0m"},
		{"Dim", Dim, "hi", "\033[2mhi\033[0m"},
		{"ToolSuccess", ToolSuccess, "ok", "\033[32mok\033[0m"},
		{"ToolError", ToolError, "err", "\033[31merr\033[0m"},
		{"Warning", Warning, "warn", "\033[33mwarn\033[0m"},
		{"Muted", Muted, "info", "\033[90minfo\033[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.in)
			if got != tt.want {
				t.Errorf("%s(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
			}
		})
	}
}

func TestStylesNoColor(t *testing.T) {
	profile = NoColor

	tests := []struct {
		name string
		fn   func(string) string
		in   string
	}{
		{"Bold", Bold, "hi"},
		{"Dim", Dim, "hi"},
		{"ToolSuccess", ToolSuccess, "ok"},
		{"ToolError", ToolError, "err"},
		{"Warning", Warning, "warn"},
		{"Muted", Muted, "info"},
		{"ToolName", ToolName, "read"},
		{"Banner", Banner, "title"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.in)
			if got != tt.in {
				t.Errorf("%s(%q) with NoColor = %q, want %q (plain)", tt.name, tt.in, got, tt.in)
			}
		})
	}
}

func TestToolName(t *testing.T) {
	profile = ANSI16
	got := ToolName("read")
	// cyan (36) + bold (1)
	want := "\033[36m\033[1mread\033[0m"
	if got != want {
		t.Errorf("ToolName(\"read\") = %q, want %q", got, want)
	}
}

func TestAccessibleIconsNoColor(t *testing.T) {
	profile = NoColor
	if ToolIcon() != "[TOOL] " {
		t.Errorf("ToolIcon NoColor = %q", ToolIcon())
	}
	if SuccessIcon() != "[OK]" {
		t.Errorf("SuccessIcon NoColor = %q", SuccessIcon())
	}
	if ErrorIcon() != "[ERR]" {
		t.Errorf("ErrorIcon NoColor = %q", ErrorIcon())
	}
	if CodeBorder() != "|" {
		t.Errorf("CodeBorder NoColor = %q", CodeBorder())
	}
}

func TestAccessibleIconsColor(t *testing.T) {
	profile = ANSI16
	if ToolIcon() != "⚡ " {
		t.Errorf("ToolIcon Color = %q", ToolIcon())
	}
	if SuccessIcon() != "✓" {
		t.Errorf("SuccessIcon Color = %q", SuccessIcon())
	}
	if ErrorIcon() != "✗" {
		t.Errorf("ErrorIcon Color = %q", ErrorIcon())
	}
	if CodeBorder() != "\u2502" {
		t.Errorf("CodeBorder Color = %q", CodeBorder())
	}
}

func TestPrompt(t *testing.T) {
	profile = ANSI16
	got := Prompt()
	want := "\033[1m>\033[0m "
	if got != want {
		t.Errorf("Prompt() = %q, want %q", got, want)
	}

	profile = NoColor
	got = Prompt()
	want = "> "
	if got != want {
		t.Errorf("Prompt() NoColor = %q, want %q", got, want)
	}
}
