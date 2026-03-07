package ui

import (
	"strings"
	"testing"
)

func TestTableEmpty(t *testing.T) {
	tbl := NewTable()
	if tbl.Render() != "" {
		t.Error("empty table should render empty string")
	}
}

func TestTableHeaderless(t *testing.T) {
	tbl := NewTable()
	tbl.AddRow("alpha", "one")
	tbl.AddRow("beta", "two")

	out := tbl.Render()
	if !strings.Contains(out, "alpha") {
		t.Error("missing 'alpha' in output")
	}
	if !strings.Contains(out, "two") {
		t.Error("missing 'two' in output")
	}
	// Should not contain separator
	if strings.Contains(out, "\u2500") && profile != NoColor {
		t.Error("headerless table should not have separator")
	}
}

func TestTableWithHeaders(t *testing.T) {
	profile = ANSI16

	tbl := NewTable("Name", "Value")
	tbl.AddRow("foo", "bar")
	tbl.AddRow("longer", "x")

	out := tbl.Render()
	if !strings.Contains(out, "Name") {
		t.Error("missing header 'Name'")
	}
	if !strings.Contains(out, "\u2500") {
		t.Error("missing separator line")
	}
	if !strings.Contains(out, "foo") {
		t.Error("missing 'foo' in output")
	}
}

func TestTableNoColorSeparator(t *testing.T) {
	profile = NoColor

	tbl := NewTable("A", "B")
	tbl.AddRow("1", "2")

	out := tbl.Render()
	if strings.Contains(out, "\u2500") {
		t.Error("NoColor should use '-' not '─'")
	}
	if !strings.Contains(out, "-") {
		t.Error("NoColor separator should be '-'")
	}
}

func TestTableColumnAlignment(t *testing.T) {
	profile = NoColor

	tbl := NewTable()
	tbl.AddRow("short", "description one")
	tbl.AddRow("much longer name", "description two")

	out := tbl.Render()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Second column should start at same position
	idx1 := strings.Index(lines[0], "description one")
	idx2 := strings.Index(lines[1], "description two")
	if idx1 != idx2 {
		t.Errorf("columns not aligned: %d vs %d", idx1, idx2)
	}
}

func TestTableSetPadding(t *testing.T) {
	profile = NoColor
	tbl := NewTable()
	tbl.SetPadding(4)
	tbl.AddRow("a", "b")

	out := tbl.Render()
	if !strings.Contains(out, "a    b") {
		t.Errorf("expected 4-space padding, got %q", out)
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"hi", 5, "hi   "},
		{"hello", 5, "hello"},
		{"toolong", 3, "toolong"},
	}
	for _, tt := range tests {
		got := padRight(tt.s, tt.width)
		if got != tt.want {
			t.Errorf("padRight(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}
