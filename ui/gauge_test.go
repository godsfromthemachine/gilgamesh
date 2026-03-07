package ui

import (
	"strings"
	"testing"
)

func TestGaugeZero(t *testing.T) {
	profile = NoColor
	got := Gauge(0, 100, 10)
	if !strings.Contains(got, "0%") {
		t.Errorf("expected 0%%, got %q", got)
	}
	if !strings.HasPrefix(got, "[") {
		t.Errorf("expected '[' prefix, got %q", got)
	}
}

func TestGaugeFull(t *testing.T) {
	profile = NoColor
	got := Gauge(100, 100, 10)
	if !strings.Contains(got, "100%") {
		t.Errorf("expected 100%%, got %q", got)
	}
	if strings.Contains(got, ".") {
		t.Errorf("full gauge should have no empty chars, got %q", got)
	}
}

func TestGaugeHalf(t *testing.T) {
	profile = NoColor
	got := Gauge(50, 100, 10)
	if !strings.Contains(got, "50%") {
		t.Errorf("expected 50%%, got %q", got)
	}
	// Should have 5 '#' and 5 '.'
	bar := got[1:strings.Index(got, "]")]
	hashes := strings.Count(bar, "#")
	dots := strings.Count(bar, ".")
	if hashes != 5 || dots != 5 {
		t.Errorf("expected 5 hashes and 5 dots, got %d and %d", hashes, dots)
	}
}

func TestGaugeOverflow(t *testing.T) {
	profile = NoColor
	got := Gauge(200, 100, 10)
	if !strings.Contains(got, "100%") {
		t.Errorf("overflow should cap at 100%%, got %q", got)
	}
}

func TestGaugeColorGreen(t *testing.T) {
	profile = ANSI16
	got := Gauge(30, 100, 10)
	if !strings.Contains(got, "\033[32m") {
		t.Error("expected green color for <50%")
	}
}

func TestGaugeColorYellow(t *testing.T) {
	profile = ANSI16
	got := Gauge(60, 100, 10)
	if !strings.Contains(got, "\033[33m") {
		t.Error("expected yellow color for 50-80%")
	}
}

func TestGaugeColorRed(t *testing.T) {
	profile = ANSI16
	got := Gauge(90, 100, 10)
	if !strings.Contains(got, "\033[31m") {
		t.Error("expected red color for >=80%")
	}
}

func TestFormatK(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{500, "500"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{12000, "12.0K"},
	}
	for _, tt := range tests {
		got := formatK(tt.n)
		if got != tt.want {
			t.Errorf("formatK(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
