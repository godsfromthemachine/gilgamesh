package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Read Tool Tests ---

func TestReadToolExistingFile(t *testing.T) {
	tool := ReadTool()

	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string) string
		args      func(path string) json.RawMessage
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "read existing file with numbered lines",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "hello.txt")
				os.WriteFile(p, []byte("line one\nline two\nline three\n"), 0644)
				return p
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{"path": path})
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "1│line one") {
					t.Errorf("expected numbered line 1, got:\n%s", result)
				}
				if !strings.Contains(result, "2│line two") {
					t.Errorf("expected numbered line 2, got:\n%s", result)
				}
				if !strings.Contains(result, "3│line three") {
					t.Errorf("expected numbered line 3, got:\n%s", result)
				}
			},
		},
		{
			name: "read with offset and limit",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "lines.txt")
				var b strings.Builder
				for i := 1; i <= 20; i++ {
					fmt.Fprintf(&b, "line %d\n", i)
				}
				os.WriteFile(p, []byte(b.String()), 0644)
				return p
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{"path": path, "offset": 5, "limit": 3})
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "5│line 5") {
					t.Errorf("expected line 5 at offset, got:\n%s", result)
				}
				if !strings.Contains(result, "7│line 7") {
					t.Errorf("expected line 7 as last in limit, got:\n%s", result)
				}
				if strings.Contains(result, "8│line 8") {
					t.Errorf("should not contain line 8 (beyond limit), got:\n%s", result)
				}
			},
		},
		{
			name: "read nonexistent file returns error",
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "does_not_exist.txt")
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{"path": path})
			},
			wantErr: true,
		},
		{
			name: "read empty file",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "empty.txt")
				os.WriteFile(p, []byte(""), 0644)
				return p
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{"path": path})
			},
			checkFunc: func(t *testing.T, result string) {
				// Empty file split produces [""], so one line with empty content
				if !strings.Contains(result, "1│") {
					t.Errorf("expected at least line 1 marker, got:\n%q", result)
				}
			},
		},
		{
			name: "read file exceeding 500-line cap",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "big.txt")
				var b strings.Builder
				for i := 1; i <= 600; i++ {
					fmt.Fprintf(&b, "line %d\n", i)
				}
				os.WriteFile(p, []byte(b.String()), 0644)
				return p
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{"path": path})
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "... (") {
					t.Errorf("expected truncation notice, got tail:\n%s", result[len(result)-100:])
				}
				if !strings.Contains(result, "500│line 500") {
					t.Errorf("expected line 500 in output")
				}
				if strings.Contains(result, "501│line 501") {
					t.Errorf("should not contain line 501 (capped at 500)")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setup(t, dir)
			args := tt.args(path)

			result, err := tool.Execute(args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

// --- Write Tool Tests ---

func TestWriteToolCases(t *testing.T) {
	tool := WriteTool()

	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		args      func(dir string) json.RawMessage
		wantErr   bool
		checkFunc func(t *testing.T, dir string, result string)
	}{
		{
			name:  "write new file",
			setup: func(t *testing.T, dir string) {},
			args: func(dir string) json.RawMessage {
				return toJSON(map[string]any{
					"path":    filepath.Join(dir, "new.txt"),
					"content": "hello world",
				})
			},
			checkFunc: func(t *testing.T, dir string, result string) {
				data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
				if err != nil {
					t.Fatalf("file not created: %v", err)
				}
				if string(data) != "hello world" {
					t.Errorf("content = %q, want %q", string(data), "hello world")
				}
				if !strings.Contains(result, "11 bytes") {
					t.Errorf("result should mention byte count, got: %s", result)
				}
			},
		},
		{
			name: "overwrite existing file",
			setup: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("old content"), 0644)
			},
			args: func(dir string) json.RawMessage {
				return toJSON(map[string]any{
					"path":    filepath.Join(dir, "existing.txt"),
					"content": "new content",
				})
			},
			checkFunc: func(t *testing.T, dir string, result string) {
				data, _ := os.ReadFile(filepath.Join(dir, "existing.txt"))
				if string(data) != "new content" {
					t.Errorf("content = %q, want %q", string(data), "new content")
				}
			},
		},
		{
			name:  "write with auto-mkdir nested dirs",
			setup: func(t *testing.T, dir string) {},
			args: func(dir string) json.RawMessage {
				return toJSON(map[string]any{
					"path":    filepath.Join(dir, "a", "b", "c", "deep.txt"),
					"content": "deep content",
				})
			},
			checkFunc: func(t *testing.T, dir string, result string) {
				data, err := os.ReadFile(filepath.Join(dir, "a", "b", "c", "deep.txt"))
				if err != nil {
					t.Fatalf("file not created in nested dir: %v", err)
				}
				if string(data) != "deep content" {
					t.Errorf("content = %q, want %q", string(data), "deep content")
				}
			},
		},
		{
			name:  "write empty content",
			setup: func(t *testing.T, dir string) {},
			args: func(dir string) json.RawMessage {
				return toJSON(map[string]any{
					"path":    filepath.Join(dir, "empty.txt"),
					"content": "",
				})
			},
			checkFunc: func(t *testing.T, dir string, result string) {
				data, err := os.ReadFile(filepath.Join(dir, "empty.txt"))
				if err != nil {
					t.Fatalf("file not created: %v", err)
				}
				if len(data) != 0 {
					t.Errorf("expected empty file, got %d bytes", len(data))
				}
				if !strings.Contains(result, "0 bytes") {
					t.Errorf("result should mention 0 bytes, got: %s", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			args := tt.args(dir)

			result, err := tool.Execute(args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, dir, result)
			}
		})
	}
}

// --- Edit Tool Tests ---

func TestEditToolCases(t *testing.T) {
	tool := EditTool()

	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string) string
		args      func(path string) json.RawMessage
		wantErr   bool
		errSubstr string
		checkFunc func(t *testing.T, path string, result string)
	}{
		{
			name: "successful find and replace",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "code.go")
				os.WriteFile(p, []byte("func hello() {\n\tfmt.Println(\"hi\")\n}\n"), 0644)
				return p
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{
					"path":       path,
					"old_string": "fmt.Println(\"hi\")",
					"new_string": "fmt.Println(\"hello world\")",
				})
			},
			checkFunc: func(t *testing.T, path string, result string) {
				data, _ := os.ReadFile(path)
				if !strings.Contains(string(data), "hello world") {
					t.Errorf("replacement not applied, file content: %s", string(data))
				}
				if !strings.Contains(result, "Edited") {
					t.Errorf("expected 'Edited' in result, got: %s", result)
				}
			},
		},
		{
			name: "no match found returns error",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "code.go")
				os.WriteFile(p, []byte("func main() {}\n"), 0644)
				return p
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{
					"path":       path,
					"old_string": "this does not exist",
					"new_string": "replacement",
				})
			},
			wantErr:   true,
			errSubstr: "not found",
		},
		{
			name: "multiple matches returns error",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "dup.txt")
				os.WriteFile(p, []byte("hello\nworld\nhello\n"), 0644)
				return p
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{
					"path":       path,
					"old_string": "hello",
					"new_string": "goodbye",
				})
			},
			wantErr:   true,
			errSubstr: "2 times",
		},
		{
			name: "replace with empty string deletes text",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "del.txt")
				os.WriteFile(p, []byte("keep this\nremove this line\nkeep this too\n"), 0644)
				return p
			},
			args: func(path string) json.RawMessage {
				return toJSON(map[string]any{
					"path":       path,
					"old_string": "remove this line\n",
					"new_string": "",
				})
			},
			checkFunc: func(t *testing.T, path string, result string) {
				data, _ := os.ReadFile(path)
				content := string(data)
				if strings.Contains(content, "remove this line") {
					t.Errorf("deleted text still present: %s", content)
				}
				if !strings.Contains(content, "keep this") || !strings.Contains(content, "keep this too") {
					t.Errorf("kept text was removed: %s", content)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setup(t, dir)
			args := tt.args(path)

			result, err := tool.Execute(args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, path, result)
			}
		})
	}
}

// --- Glob Tool Tests ---

func TestGlobToolCases(t *testing.T) {
	tool := GlobTool()

	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		pattern   func(dir string) string
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "match *.go files",
			setup: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
				os.WriteFile(filepath.Join(dir, "util.go"), []byte("package main"), 0644)
				os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# hi"), 0644)
			},
			pattern: func(dir string) string {
				return filepath.Join(dir, "*.go")
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "main.go") {
					t.Errorf("expected main.go in results, got:\n%s", result)
				}
				if !strings.Contains(result, "util.go") {
					t.Errorf("expected util.go in results, got:\n%s", result)
				}
				if strings.Contains(result, "readme.md") {
					t.Errorf("should not contain readme.md, got:\n%s", result)
				}
			},
		},
		{
			name: "no matches",
			setup: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, "file.txt"), []byte("text"), 0644)
			},
			pattern: func(dir string) string {
				return filepath.Join(dir, "*.xyz")
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "No files found") {
					t.Errorf("expected 'No files found', got:\n%s", result)
				}
			},
		},
		{
			name: "pattern with subdirectories",
			setup: func(t *testing.T, dir string) {
				sub := filepath.Join(dir, "sub")
				os.MkdirAll(sub, 0755)
				os.WriteFile(filepath.Join(dir, "top.go"), []byte("package top"), 0644)
				os.WriteFile(filepath.Join(sub, "nested.go"), []byte("package sub"), 0644)
			},
			pattern: func(dir string) string {
				return filepath.Join(dir, "**", "*.go")
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "nested.go") {
					t.Errorf("expected nested.go in results, got:\n%s", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			pattern := tt.pattern(dir)
			args := toJSON(map[string]any{"pattern": pattern})

			result, err := tool.Execute(args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

// --- Grep Tool Tests ---

func TestGrepToolCases(t *testing.T) {
	tool := GrepTool()

	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		args      func(dir string) json.RawMessage
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "find regex match",
			setup: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, "code.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)
			},
			args: func(dir string) json.RawMessage {
				return toJSON(map[string]any{
					"pattern": "fmt\\.Println",
					"path":    dir,
				})
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "fmt.Println") {
					t.Errorf("expected match for fmt.Println, got:\n%s", result)
				}
			},
		},
		{
			name: "no matches",
			setup: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, "file.txt"), []byte("nothing relevant here\n"), 0644)
			},
			args: func(dir string) json.RawMessage {
				return toJSON(map[string]any{
					"pattern": "nonexistent_pattern_xyz",
					"path":    dir,
				})
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "No matches found") {
					t.Errorf("expected 'No matches found', got:\n%s", result)
				}
			},
		},
		{
			name: "invalid regex returns error",
			setup: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, "file.txt"), []byte("some content\n"), 0644)
			},
			args: func(dir string) json.RawMessage {
				return toJSON(map[string]any{
					"pattern": "[invalid",
					"path":    dir,
				})
			},
			wantErr: true,
		},
		{
			name: "multiple file matches",
			setup: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, "a.txt"), []byte("TODO: fix this\n"), 0644)
				os.WriteFile(filepath.Join(dir, "b.txt"), []byte("TODO: refactor\n"), 0644)
				os.WriteFile(filepath.Join(dir, "c.txt"), []byte("no match here\n"), 0644)
			},
			args: func(dir string) json.RawMessage {
				return toJSON(map[string]any{
					"pattern": "TODO",
					"path":    dir,
				})
			},
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "a.txt") {
					t.Errorf("expected a.txt in results, got:\n%s", result)
				}
				if !strings.Contains(result, "b.txt") {
					t.Errorf("expected b.txt in results, got:\n%s", result)
				}
				if strings.Contains(result, "c.txt") {
					t.Errorf("should not contain c.txt (no match), got:\n%s", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			args := tt.args(dir)

			result, err := tool.Execute(args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

// --- Test Tool Tests ---

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		expect string
	}{
		{"go project", map[string]string{"go.mod": "module test"}, "go"},
		{"rust project", map[string]string{"Cargo.toml": "[package]"}, "rust"},
		{"zig project", map[string]string{"build.zig": "const std = @import(\"std\");"}, "zig"},
		{"node project", map[string]string{"package.json": "{}"}, "node"},
		{"python pyproject", map[string]string{"pyproject.toml": "[project]"}, "python"},
		{"python setup.py", map[string]string{"setup.py": "from setuptools import setup"}, "python"},
		{"python pytest.ini", map[string]string{"pytest.ini": "[pytest]"}, "python"},
		{"python requirements", map[string]string{"requirements.txt": "flask"}, "python"},
		{"go takes priority over node", map[string]string{"go.mod": "module test", "package.json": "{}"}, "go"},
		{"empty dir defaults to go", map[string]string{}, "go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
			}
			// Change to temp dir so detectLanguage reads the right files
			orig, _ := os.Getwd()
			os.Chdir(dir)
			defer os.Chdir(orig)

			got := detectLanguage()
			if got != tt.expect {
				t.Errorf("detectLanguage() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestTestToolUnsupportedLanguage(t *testing.T) {
	tool := TestTool()
	result, err := tool.Execute(toJSON(map[string]any{"language": "cobol"}))
	if err == nil {
		t.Fatalf("expected error for unsupported language, got result: %s", result)
	}
	if !strings.Contains(err.Error(), "unsupported language") {
		t.Errorf("error should mention unsupported language, got: %s", err)
	}
}

func TestTestToolGoOnTempProject(t *testing.T) {
	dir := t.TempDir()
	// Create a minimal Go module with a passing test
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "add.go"), []byte("package testmod\n\nfunc Add(a, b int) int { return a + b }\n"), 0644)
	os.WriteFile(filepath.Join(dir, "add_test.go"), []byte("package testmod\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 { t.Fatal(\"bad\") }\n}\n"), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	tool := TestTool()
	result, err := tool.Execute(toJSON(map[string]any{"language": "go", "package": "."}))
	if err != nil {
		t.Fatalf("test tool error: %v", err)
	}
	if !strings.HasPrefix(result, "[go]") {
		t.Errorf("result should start with [go], got: %q", result[:min(50, len(result))])
	}
	if !strings.Contains(result, "ok") {
		t.Errorf("expected passing test output, got:\n%s", result)
	}
}

func TestTestToolGoFailingTest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "bad_test.go"), []byte("package testmod\n\nimport \"testing\"\n\nfunc TestBad(t *testing.T) { t.Fatal(\"intentional\") }\n"), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	tool := TestTool()
	result, err := tool.Execute(toJSON(map[string]any{"language": "go"}))
	if err != nil {
		t.Fatalf("test tool should not return error for test failures: %v", err)
	}
	if !strings.Contains(result, "FAIL") {
		t.Errorf("expected FAIL in output, got:\n%s", result)
	}
}

func TestTestToolAutoDetect(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("package testmod\n"), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	tool := TestTool()
	// No language specified — should auto-detect Go
	result, err := tool.Execute(toJSON(map[string]any{}))
	if err != nil {
		t.Fatalf("test tool error: %v", err)
	}
	if !strings.HasPrefix(result, "[go]") {
		t.Errorf("auto-detected result should start with [go], got: %q", result[:min(50, len(result))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- Helper ---

func toJSON(m map[string]any) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}
