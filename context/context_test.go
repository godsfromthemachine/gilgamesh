package context

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenEstimate(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"test", 1},
		{"hello world!!", 3}, // 13/4 = 3
		{"a", 0},             // 1/4 = 0
	}
	for _, tt := range tests {
		got := TokenEstimate(tt.input)
		if got != tt.want {
			t.Errorf("TokenEstimate(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestLoadFromGilgameshfile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.WriteFile(".gilgameshfile", []byte("project context here"), 0644)

	ctx := Load()
	if ctx != "project context here" {
		t.Errorf("Load() = %q, want 'project context here'", ctx)
	}
}

func TestLoadFromContextMd(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll(".gilgamesh", 0755)
	os.WriteFile(".gilgamesh/context.md", []byte("context from md"), 0644)

	ctx := Load()
	if ctx != "context from md" {
		t.Errorf("Load() = %q, want 'context from md'", ctx)
	}
}

func TestLoadGilgameshfilePrecedence(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.WriteFile(".gilgameshfile", []byte("from file"), 0644)
	os.MkdirAll(".gilgamesh", 0755)
	os.WriteFile(".gilgamesh/context.md", []byte("from md"), 0644)

	ctx := Load()
	if ctx != "from file" {
		t.Errorf("Load() = %q, want 'from file' (.gilgameshfile takes precedence)", ctx)
	}
}

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	ctx := Load()
	if ctx != "" {
		t.Errorf("Load() = %q, want empty string", ctx)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.WriteFile(".gilgameshfile", []byte("   \n  \n  "), 0644)

	ctx := Load()
	if ctx != "" {
		t.Errorf("Load() = %q, want empty for whitespace-only file", ctx)
	}
}

func TestParseSkill(t *testing.T) {
	content := "# Build and test\nBuild the project and run all tests."
	s := parseSkill("build", content)

	if s.Name != "build" {
		t.Errorf("Name = %q, want build", s.Name)
	}
	if s.Description != "Build and test" {
		t.Errorf("Description = %q, want 'Build and test'", s.Description)
	}
	if s.Prompt != content {
		t.Errorf("Prompt not preserved")
	}
}

func TestFormatSkillPrompt(t *testing.T) {
	skill := Skill{Prompt: "Test: {{args}}"}

	got := FormatSkillPrompt(skill, "my function")
	if got != "Test: my function" {
		t.Errorf("got %q, want 'Test: my function'", got)
	}

	got = FormatSkillPrompt(skill, "")
	if got != "Test: " {
		t.Errorf("got %q, want 'Test: '", got)
	}
}

func TestListSkillsEmpty(t *testing.T) {
	got := ListSkills(map[string]Skill{})
	if got == "" {
		t.Error("ListSkills should return help text for empty map")
	}
}

func TestListSkillsNonEmpty(t *testing.T) {
	skills := map[string]Skill{
		"test": {Name: "test", Description: "Run tests"},
	}
	got := ListSkills(skills)
	if !containsStr(got, "/test") || !containsStr(got, "Run tests") {
		t.Errorf("ListSkills output missing expected content: %s", got)
	}
}

func TestLoadSkillsFromDir(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	skillDir := filepath.Join(dir, ".gilgamesh", "skills")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "custom.md"), []byte("# Custom skill\nDo custom thing: {{args}}"), 0644)

	skills := LoadSkills()
	if _, ok := skills["custom"]; !ok {
		t.Error("missing custom skill")
	}
	if skills["custom"].Description != "Custom skill" {
		t.Errorf("Description = %q, want 'Custom skill'", skills["custom"].Description)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
