package ui

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner shows an animated progress indicator during tool execution.
type Spinner struct {
	message   string
	done      chan struct{}
	mu        sync.Mutex
	running   bool
	startTime time.Time
}

// NewSpinner creates a spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		done:    make(chan struct{}),
	}
}

// Start begins the spinner animation in a goroutine.
// Does nothing if stdout is not a TTY.
func (s *Spinner) Start() {
	if !IsTerminal(os.Stdout.Fd()) {
		return
	}

	s.mu.Lock()
	s.running = true
	s.startTime = time.Now()
	s.mu.Unlock()

	go func() {
		i := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				frame := spinnerFrames[i%len(spinnerFrames)]
				elapsed := time.Since(s.startTime)
				suffix := ""
				if elapsed >= 500*time.Millisecond {
					suffix = Dim(fmt.Sprintf(" [%.1fs]", elapsed.Seconds()))
				}
				fmt.Fprintf(os.Stderr, "\r%s %s%s\033[K", frame, s.message, suffix)
				i++
			}
		}
	}()
}

// Stop stops the spinner and prints the final result line.
func (s *Spinner) Stop(result string) {
	s.mu.Lock()
	wasRunning := s.running
	s.running = false
	s.mu.Unlock()

	if wasRunning {
		close(s.done)
		// Clear the spinner line
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}

	if result != "" {
		fmt.Fprintln(os.Stderr, result)
	}
}
