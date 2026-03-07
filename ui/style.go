package ui

import "fmt"

// ANSI escape helpers — all return plain text when profile == NoColor.

const (
	esc   = "\033["
	reset = "\033[0m"
)

// Bold wraps s in bold ANSI codes.
func Bold(s string) string {
	if profile == NoColor {
		return s
	}
	return esc + "1m" + s + reset
}

// Dim wraps s in dim ANSI codes.
func Dim(s string) string {
	if profile == NoColor {
		return s
	}
	return esc + "2m" + s + reset
}

// Fg wraps s with a foreground color code (0-255).
func Fg(code int, s string) string {
	if profile == NoColor {
		return s
	}
	return fmt.Sprintf("%s%dm%s%s", esc, code, s, reset)
}

// fgBold wraps s with foreground color + bold.
func fgBold(code int, s string) string {
	if profile == NoColor {
		return s
	}
	return fmt.Sprintf("%s%dm%s1m%s%s", esc, code, esc, s, reset)
}

// --- Semantic styles (used throughout gilgamesh) ---

// ToolName styles a tool name: cyan bold.
func ToolName(s string) string {
	return fgBold(36, s)
}

// ToolSuccess styles success output: green.
func ToolSuccess(s string) string {
	return Fg(32, s)
}

// ToolError styles error output: red.
func ToolError(s string) string {
	return Fg(31, s)
}

// Warning styles warning text: yellow.
func Warning(s string) string {
	return Fg(33, s)
}

// Muted styles secondary/informational text: dim gray (90).
func Muted(s string) string {
	if profile == NoColor {
		return s
	}
	return esc + "90m" + s + reset
}

// Prompt returns the REPL prompt string.
func Prompt() string {
	return Bold(">") + " "
}

// Banner styles the startup title: bold.
func Banner(s string) string {
	return Bold(s)
}

// --- Accessible icon functions (NoColor-safe) ---

// ToolIcon returns "⚡ " or "[TOOL] " based on color profile.
func ToolIcon() string {
	if profile == NoColor {
		return "[TOOL] "
	}
	return "⚡ "
}

// SuccessIcon returns "✓" or "[OK]" based on color profile.
func SuccessIcon() string {
	if profile == NoColor {
		return "[OK]"
	}
	return "✓"
}

// ErrorIcon returns "✗" or "[ERR]" based on color profile.
func ErrorIcon() string {
	if profile == NoColor {
		return "[ERR]"
	}
	return "✗"
}

// CodeBorder returns "│" or "|" based on color profile.
func CodeBorder() string {
	if profile == NoColor {
		return "|"
	}
	return "\u2502"
}
