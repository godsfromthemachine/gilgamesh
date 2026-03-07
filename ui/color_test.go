package ui

import (
	"os"
	"testing"
)

func TestDetectProfile(t *testing.T) {
	envKeys := []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "COLORTERM", "TERM"}

	tests := []struct {
		name string
		env  map[string]string
		want Profile
	}{
		{
			name: "NO_COLOR set",
			env:  map[string]string{"NO_COLOR": ""},
			want: NoColor,
		},
		{
			name: "NO_COLOR with value",
			env:  map[string]string{"NO_COLOR": "1"},
			want: NoColor,
		},
		{
			name: "CLICOLOR=0",
			env:  map[string]string{"CLICOLOR": "0"},
			want: NoColor,
		},
		{
			name: "CLICOLOR_FORCE=1 with truecolor",
			env:  map[string]string{"CLICOLOR_FORCE": "1", "COLORTERM": "truecolor"},
			want: TrueColor,
		},
		{
			name: "CLICOLOR_FORCE=1 with 24bit",
			env:  map[string]string{"CLICOLOR_FORCE": "1", "COLORTERM": "24bit"},
			want: TrueColor,
		},
		{
			name: "CLICOLOR_FORCE=1 with 256color term",
			env:  map[string]string{"CLICOLOR_FORCE": "1", "TERM": "xterm-256color"},
			want: ANSI256,
		},
		{
			name: "CLICOLOR_FORCE=1 default",
			env:  map[string]string{"CLICOLOR_FORCE": "1"},
			want: ANSI16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unset all relevant env vars first
			for _, key := range envKeys {
				os.Unsetenv(key)
			}
			// Set only the test env vars (t.Setenv handles restore)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got := DetectProfile()
			if got != tt.want {
				t.Errorf("DetectProfile() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTerminalWidth(t *testing.T) {
	w := TerminalWidth()
	if w <= 0 {
		t.Errorf("TerminalWidth() = %d, want > 0", w)
	}
}

func TestProfileConstants(t *testing.T) {
	if NoColor != 0 {
		t.Errorf("NoColor = %d, want 0", NoColor)
	}
	if ANSI16 != 1 {
		t.Errorf("ANSI16 = %d, want 1", ANSI16)
	}
	if ANSI256 != 2 {
		t.Errorf("ANSI256 = %d, want 2", ANSI256)
	}
	if TrueColor != 3 {
		t.Errorf("TrueColor = %d, want 3", TrueColor)
	}
}
