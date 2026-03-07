package ui

import "testing"

func TestNewSpinner(t *testing.T) {
	s := NewSpinner("testing")
	if s == nil {
		t.Fatal("NewSpinner returned nil")
	}
	if s.message != "testing" {
		t.Errorf("message = %q, want %q", s.message, "testing")
	}
	if s.running {
		t.Error("spinner should not be running before Start()")
	}
}

func TestSpinnerStopWithoutStart(t *testing.T) {
	// Calling Stop without Start should not panic.
	s := NewSpinner("test")
	s.Stop("done")
}

func TestSpinnerStartStop(t *testing.T) {
	// Start + Stop should not panic or hang, even in test (non-TTY) env.
	s := NewSpinner("working")
	s.Start() // no-op in non-TTY (test environment)
	s.Stop("result")
}

func TestSpinnerFrames(t *testing.T) {
	if len(spinnerFrames) == 0 {
		t.Error("spinnerFrames is empty")
	}
}
