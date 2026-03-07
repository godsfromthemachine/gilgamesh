package ui

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Profile represents the terminal's color capability level.
type Profile int

const (
	NoColor   Profile = iota // No color output
	ANSI16                   // Basic 16 colors
	ANSI256                  // 256 colors
	TrueColor                // 24-bit true color
)

var profile Profile

// Init detects the terminal profile and caches it. Call once at startup.
func Init() {
	profile = DetectProfile()
}

// CurrentProfile returns the active color profile.
func CurrentProfile() Profile {
	return profile
}

// DetectProfile determines color support from env vars and TTY state.
//
// Priority:
//  1. NO_COLOR set (any value) → NoColor
//  2. CLICOLOR=0 → NoColor
//  3. CLICOLOR_FORCE=1 → force color (skip TTY check)
//  4. Not a TTY → NoColor
//  5. COLORTERM=truecolor|24bit → TrueColor
//  6. TERM contains "256color" → ANSI256
//  7. Default TTY → ANSI16
func DetectProfile() Profile {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return NoColor
	}
	if os.Getenv("CLICOLOR") == "0" {
		return NoColor
	}

	forced := os.Getenv("CLICOLOR_FORCE") == "1"
	if !forced && !IsTerminal(os.Stdout.Fd()) {
		return NoColor
	}

	ct := os.Getenv("COLORTERM")
	if ct == "truecolor" || ct == "24bit" {
		return TrueColor
	}
	if strings.Contains(os.Getenv("TERM"), "256color") {
		return ANSI256
	}
	return ANSI16
}

// IsTerminal reports whether the given file descriptor refers to a terminal.
func IsTerminal(fd uintptr) bool {
	var ws struct{ Row, Col, Xpixel, Ypixel uint16 }
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	return err == 0
}

// TerminalWidth returns the terminal width in columns, defaulting to 80.
func TerminalWidth() int {
	var ws struct{ Row, Col, Xpixel, Ypixel uint16 }
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if err != 0 || ws.Col == 0 {
		return 80
	}
	return int(ws.Col)
}
