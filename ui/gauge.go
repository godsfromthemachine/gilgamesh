package ui

import "fmt"

// Gauge renders a text progress bar: [████████░░░░░░░░] 8.2K/12K (68%)
func Gauge(current, max, width int) string {
	if max <= 0 {
		max = 1
	}
	pct := current * 100 / max
	if pct > 100 {
		pct = 100
	}

	filled := width * current / max
	if filled > width {
		filled = width
	}
	empty := width - filled

	// Choose bar characters
	fillChar, emptyChar := "#", "."
	if profile != NoColor {
		fillChar, emptyChar = "\u2588", "\u2591"
	}

	bar := ""
	for range filled {
		bar += fillChar
	}
	for range empty {
		bar += emptyChar
	}

	// Format counts as K if >= 1000
	label := fmt.Sprintf("%s/%s (%d%%)", formatK(current), formatK(max), pct)

	// Color based on percentage
	if profile == NoColor {
		return "[" + bar + "] " + label
	}

	if pct >= 80 {
		return "[" + Fg(31, bar) + "] " + label // red
	}
	if pct >= 50 {
		return "[" + Fg(33, bar) + "] " + label // yellow
	}
	return "[" + Fg(32, bar) + "] " + label // green
}

func formatK(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
