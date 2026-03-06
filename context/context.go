package context

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed builtin_skills/*.md
var builtinSkillsFS embed.FS

// Load reads project context from .gilgameshfile or .gilgamesh/context.md.
// Returns empty string if no context file is found.
func Load() string {
	paths := []string{
		".gilgameshfile",
		".gilgamesh/context.md",
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			return content
		}
	}

	return ""
}

// TokenEstimate returns a rough token count for the context.
func TokenEstimate(s string) int {
	return len(s) / 4
}

// LoadSkills reads skills from three sources in priority order:
//  1. Built-in skills (embedded in binary, lowest priority)
//  2. Global skills (~/.config/gilgamesh/skills/)
//  3. Project-local skills (.gilgamesh/skills/, highest priority)
//
// Higher-priority skills override lower-priority ones with the same name.
func LoadSkills() map[string]Skill {
	skills := make(map[string]Skill)

	// 1. Load built-in skills (lowest priority)
	entries, err := builtinSkillsFS.ReadDir("builtin_skills")
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			data, err := builtinSkillsFS.ReadFile("builtin_skills/" + e.Name())
			if err != nil {
				continue
			}
			s := parseSkill(name, string(data))
			s.Builtin = true
			skills[name] = s
		}
	}

	// 2. Load global skills (medium priority — overrides built-in)
	if home, err := os.UserHomeDir(); err == nil {
		globalDir := filepath.Join(home, ".config", "gilgamesh", "skills")
		loadSkillDir(globalDir, skills, false)
	}

	// 3. Load project-local skills (highest priority — overrides everything)
	loadSkillDir(".gilgamesh/skills", skills, false)

	return skills
}

// loadSkillDir reads .md files from a directory and adds them to the skills map.
// Existing entries are overwritten (higher priority caller wins).
func loadSkillDir(dir string, skills map[string]Skill, builtin bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		s := parseSkill(name, string(data))
		s.Builtin = builtin
		skills[name] = s
	}
}

// Skill represents a reusable prompt template.
type Skill struct {
	Name        string
	Description string // first line of the file
	Prompt      string // full content used as the user message
	Builtin     bool   // true if embedded in the binary
}

func parseSkill(name, content string) Skill {
	content = strings.TrimSpace(content)
	s := Skill{Name: name, Prompt: content}

	// First line starting with # or plain text is the description
	lines := strings.SplitN(content, "\n", 2)
	desc := strings.TrimLeft(lines[0], "# ")
	s.Description = desc

	return s
}

// FormatSkillPrompt replaces {{args}} in the skill prompt with the given arguments.
func FormatSkillPrompt(skill Skill, args string) string {
	prompt := skill.Prompt
	if args != "" {
		prompt = strings.ReplaceAll(prompt, "{{args}}", args)
	} else {
		prompt = strings.ReplaceAll(prompt, "{{args}}", "")
	}
	return prompt
}

// ListSkills returns a formatted string listing all available skills.
func ListSkills(skills map[string]Skill) string {
	if len(skills) == 0 {
		return "No skills found. Add .md files to .gilgamesh/skills/ or ~/.config/gilgamesh/skills/"
	}
	var sb strings.Builder
	for name, skill := range skills {
		marker := ""
		if skill.Builtin {
			marker = " (built-in)"
		}
		fmt.Fprintf(&sb, "  /%s — %s%s\n", name, skill.Description, marker)
	}
	return sb.String()
}

// CountSkills returns the number of built-in and project/global skills.
func CountSkills(skills map[string]Skill) (builtin, custom int) {
	for _, s := range skills {
		if s.Builtin {
			builtin++
		} else {
			custom++
		}
	}
	return
}
