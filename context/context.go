package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// LoadSkills reads skill files from .gilgamesh/skills/ and ~/.config/gilgamesh/skills/.
// Returns a map of skill name -> skill content.
func LoadSkills() map[string]Skill {
	skills := make(map[string]Skill)

	dirs := []string{".gilgamesh/skills"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "gilgamesh", "skills"))
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			if _, exists := skills[name]; exists {
				continue // project-local takes precedence
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			skills[name] = parseSkill(name, string(data))
		}
	}

	return skills
}

// Skill represents a reusable prompt template.
type Skill struct {
	Name        string
	Description string // first line of the file
	Prompt      string // full content used as the user message
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
		fmt.Fprintf(&sb, "  /%s — %s\n", name, skill.Description)
	}
	return sb.String()
}
