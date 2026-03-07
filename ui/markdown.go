package ui

import "strings"

// StreamRenderer processes streaming LLM tokens and formats code blocks
// with visual distinction (left border, language label, keyword highlighting).
type StreamRenderer struct {
	inCodeBlock bool
	codeLang    string
	lineBuf     strings.Builder
	profile     Profile
}

// NewStreamRenderer creates a new markdown stream renderer.
func NewStreamRenderer() *StreamRenderer {
	return &StreamRenderer{profile: profile}
}

// WriteToken processes a streaming token and returns formatted output.
func (r *StreamRenderer) WriteToken(token string) string {
	if r.profile == NoColor {
		return token
	}

	var out strings.Builder
	for _, ch := range token {
		r.lineBuf.WriteRune(ch)
		if ch == '\n' {
			out.WriteString(r.processLine(r.lineBuf.String()))
			r.lineBuf.Reset()
		}
	}
	return out.String()
}

// Flush returns any remaining buffered content.
func (r *StreamRenderer) Flush() string {
	if r.lineBuf.Len() == 0 {
		return ""
	}
	line := r.lineBuf.String()
	r.lineBuf.Reset()

	if r.profile == NoColor {
		return line
	}

	return r.processLine(line)
}

// processLine handles a complete line, managing code block state.
func (r *StreamRenderer) processLine(line string) string {
	trimmed := strings.TrimSpace(line)

	// Toggle code block on ``` fence
	if strings.HasPrefix(trimmed, "```") {
		if !r.inCodeBlock {
			r.inCodeBlock = true
			r.codeLang = strings.TrimPrefix(trimmed, "```")
			r.codeLang = strings.TrimSpace(r.codeLang)
			// Print language label
			if r.codeLang != "" {
				return Muted("  " + r.codeLang) + "\n"
			}
			return "\n"
		}
		// Closing fence
		r.inCodeBlock = false
		r.codeLang = ""
		return "\n"
	}

	if r.inCodeBlock {
		return r.renderCodeLine(line)
	}

	// Horizontal rule: ---, ***, ___
	stripped := strings.TrimSpace(strings.TrimRight(line, "\n"))
	if len(stripped) >= 3 && (allChar(stripped, '-') || allChar(stripped, '*') || allChar(stripped, '_')) {
		return Muted(strings.Repeat("─", 60)) + "\n"
	}

	// Headers: # , ## , ### etc.
	if strings.HasPrefix(trimmed, "# ") {
		return Bold(strings.TrimPrefix(trimmed, "# ")) + "\n"
	}
	if strings.HasPrefix(trimmed, "## ") {
		return Bold(strings.TrimPrefix(trimmed, "## ")) + "\n"
	}
	if strings.HasPrefix(trimmed, "### ") {
		return Bold(strings.TrimPrefix(trimmed, "### ")) + "\n"
	}

	// Blockquote: > text
	if strings.HasPrefix(trimmed, "> ") {
		return Muted("  > ") + renderInline(strings.TrimPrefix(trimmed, "> ")) + "\n"
	}

	// Unordered list: - , * , +
	if (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ")) && len(trimmed) > 2 {
		return "  • " + renderInline(trimmed[2:]) + "\n"
	}

	// Ordered list: 1. , 2. , etc.
	if len(trimmed) >= 3 {
		dotIdx := strings.Index(trimmed, ". ")
		if dotIdx > 0 && dotIdx <= 3 {
			prefix := trimmed[:dotIdx]
			allDigits := true
			for _, c := range prefix {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return "  " + prefix + ". " + renderInline(trimmed[dotIdx+2:]) + "\n"
			}
		}
	}

	// Inline formatting for regular text
	hasNewline := strings.HasSuffix(line, "\n")
	result := renderInline(strings.TrimRight(line, "\n"))
	if hasNewline {
		result += "\n"
	}
	return result
}

// allChar returns true if s consists entirely of the given character.
func allChar(s string, c byte) bool {
	for i := range len(s) {
		if s[i] != c {
			return false
		}
	}
	return true
}

// renderInline applies bold and italic inline formatting.
func renderInline(s string) string {
	if profile == NoColor {
		return s
	}

	var out strings.Builder
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		// Bold: **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			end := findClosing(runes, i+2, "**")
			if end >= 0 {
				out.WriteString(Bold(string(runes[i+2 : end])))
				i = end + 2
				continue
			}
		}
		// Italic: *text*
		if runes[i] == '*' {
			end := findClosingSingle(runes, i+1, '*')
			if end >= 0 {
				out.WriteString(Dim(string(runes[i+1 : end])))
				i = end + 1
				continue
			}
		}
		// Inline code: `text`
		if runes[i] == '`' {
			end := findClosingSingle(runes, i+1, '`')
			if end >= 0 {
				out.WriteString(Muted(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}
		out.WriteRune(runes[i])
		i++
	}
	return out.String()
}

// findClosing finds the position of a two-char closing marker (e.g. "**").
func findClosing(runes []rune, start int, marker string) int {
	m := []rune(marker)
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == m[0] && runes[i+1] == m[1] {
			return i
		}
	}
	return -1
}

// findClosingSingle finds the position of a single-char closing marker.
func findClosingSingle(runes []rune, start int, marker rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == marker {
			return i
		}
	}
	return -1
}

// renderCodeLine adds left border and applies minimal syntax highlighting.
func (r *StreamRenderer) renderCodeLine(line string) string {
	// Strip trailing newline for processing, re-add after
	raw := strings.TrimRight(line, "\n")

	highlighted := highlightLine(raw, r.codeLang)
	return Muted("  "+CodeBorder()+" ") + highlighted + "\n"
}

// --- Minimal syntax highlighting ---

// Language keyword sets
var keywords = map[string][]string{
	"go":     {"func", "var", "const", "type", "struct", "interface", "if", "else", "for", "range", "return", "package", "import", "defer", "go", "chan", "select", "case", "switch", "map", "nil", "true", "false"},
	"python": {"def", "class", "if", "elif", "else", "for", "while", "return", "import", "from", "in", "not", "and", "or", "True", "False", "None", "with", "as", "try", "except", "raise", "yield"},
	"rust":   {"fn", "let", "mut", "const", "struct", "enum", "impl", "pub", "use", "match", "if", "else", "for", "loop", "while", "return", "self", "Self", "mod", "crate", "trait", "where"},
	"js":     {"function", "const", "let", "var", "if", "else", "for", "while", "return", "class", "new", "this", "async", "await", "import", "export", "from", "true", "false", "null", "undefined"},
	"ts":     {"function", "const", "let", "var", "if", "else", "for", "while", "return", "class", "new", "this", "async", "await", "import", "export", "from", "true", "false", "null", "undefined", "interface", "type"},
}

// Aliases for language names
var langAliases = map[string]string{
	"golang":     "go",
	"javascript": "js",
	"typescript": "ts",
	"py":         "python",
	"rs":         "rust",
}

func highlightLine(line, lang string) string {
	if lang == "" {
		return line
	}

	// Resolve alias
	if alias, ok := langAliases[lang]; ok {
		lang = alias
	}

	kws, ok := keywords[lang]
	if !ok {
		return line
	}

	// Comment detection
	commentPrefix := "//"
	if lang == "python" {
		commentPrefix = "#"
	}

	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, commentPrefix) {
		return Dim(line)
	}

	// String highlighting: find quoted regions and color them
	// Simple approach: highlight keywords that are whole words
	return highlightKeywords(line, kws)
}

// highlightKeywords bolds keywords that appear as whole words in the line.
func highlightKeywords(line string, kws []string) string {
	// Build a set for O(1) lookup
	kwSet := make(map[string]struct{}, len(kws))
	for _, kw := range kws {
		kwSet[kw] = struct{}{}
	}

	var out strings.Builder
	i := 0
	runes := []rune(line)

	for i < len(runes) {
		// Check if we're at the start of an identifier
		if isIdentStart(runes[i]) {
			j := i + 1
			for j < len(runes) && isIdentPart(runes[j]) {
				j++
			}
			word := string(runes[i:j])
			if _, ok := kwSet[word]; ok {
				// Check word boundary: previous char is not ident
				if i == 0 || !isIdentPart(runes[i-1]) {
					out.WriteString(Bold(word))
					i = j
					continue
				}
			}
			out.WriteString(word)
			i = j
			continue
		}

		// String literals: highlight in green
		if runes[i] == '"' || runes[i] == '\'' || runes[i] == '`' {
			quote := runes[i]
			j := i + 1
			for j < len(runes) && runes[j] != quote {
				if runes[j] == '\\' && j+1 < len(runes) {
					j++ // skip escaped char
				}
				j++
			}
			if j < len(runes) {
				j++ // include closing quote
			}
			out.WriteString(Fg(32, string(runes[i:j])))
			i = j
			continue
		}

		out.WriteRune(runes[i])
		i++
	}

	return out.String()
}

func isIdentStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}
