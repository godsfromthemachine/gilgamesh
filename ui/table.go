package ui

import (
	"strings"
)

// Table renders auto-sized columnar output.
type Table struct {
	headers  []string
	rows     [][]string
	padding  int
	noHeader bool
}

// NewTable creates a table with optional column headers.
// Pass no arguments for a headerless table.
func NewTable(headers ...string) *Table {
	t := &Table{padding: 2}
	if len(headers) == 0 {
		t.noHeader = true
	} else {
		t.headers = headers
	}
	return t
}

// AddRow adds a row of column values.
func (t *Table) AddRow(cols ...string) {
	t.rows = append(t.rows, cols)
}

// SetPadding sets the number of spaces between columns (default 2).
func (t *Table) SetPadding(n int) {
	t.padding = n
}

// Render returns the formatted table string.
func (t *Table) Render() string {
	if len(t.rows) == 0 && len(t.headers) == 0 {
		return ""
	}

	// Determine column count
	cols := len(t.headers)
	for _, row := range t.rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return ""
	}

	// Calculate max width per column
	widths := make([]int, cols)
	if !t.noHeader {
		for i, h := range t.headers {
			if len(h) > widths[i] {
				widths[i] = len(h)
			}
		}
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < cols && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Clamp total width to terminal width
	termW := TerminalWidth()
	totalW := 0
	for i, w := range widths {
		totalW += w
		if i < cols-1 {
			totalW += t.padding
		}
	}
	if totalW > termW && cols > 1 {
		// Shrink last column to fit
		excess := totalW - termW
		if widths[cols-1] > excess+3 {
			widths[cols-1] -= excess
		}
	}

	pad := strings.Repeat(" ", t.padding)
	var out strings.Builder

	// Header
	if !t.noHeader && len(t.headers) > 0 {
		for i, h := range t.headers {
			if i > 0 {
				out.WriteString(pad)
			}
			out.WriteString(Bold(padRight(h, widths[i])))
		}
		out.WriteString("\n")

		// Separator
		sep := "-"
		if profile != NoColor {
			sep = "\u2500"
		}
		for i := range cols {
			if i > 0 {
				out.WriteString(pad)
			}
			out.WriteString(Muted(strings.Repeat(sep, widths[i])))
		}
		out.WriteString("\n")
	}

	// Rows
	for _, row := range t.rows {
		for i := range cols {
			if i > 0 {
				out.WriteString(pad)
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if i < cols-1 {
				out.WriteString(padRight(cell, widths[i]))
			} else {
				// Last column: don't pad
				if len(cell) > widths[i] {
					out.WriteString(cell[:widths[i]-3] + "...")
				} else {
					out.WriteString(cell)
				}
			}
		}
		out.WriteString("\n")
	}

	return out.String()
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
