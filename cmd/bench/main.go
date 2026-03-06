// cmd/bench benchmarks gilgamesh against local LLM endpoints.
//
// Usage:
//
//	go run ./cmd/bench                     # bench default endpoint
//	go run ./cmd/bench -endpoint http://127.0.0.1:8081/v1
//	go run ./cmd/bench -all                # bench all configured profiles
//	go run ./cmd/bench -model heavy        # bench specific profile
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	flagEndpoint = flag.String("endpoint", "", "LLM endpoint URL (overrides config)")
	flagModel    = flag.String("model", "default", "Model profile to benchmark (fast/default/heavy)")
	flagAll      = flag.Bool("all", false, "Benchmark all reachable profiles")
	flagVerbose  = flag.Bool("v", false, "Verbose output")
)

// ANSI colors
const (
	cyan   = "\033[36m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	dim    = "\033[2m"
	reset  = "\033[0m"
)

type profile struct {
	Name     string
	Endpoint string
	APIKey   string
}

var defaultProfiles = []profile{
	{Name: "fast (2B Q4_K_M)", Endpoint: "http://127.0.0.1:8081/v1", APIKey: "sk-local"},
	{Name: "heavy (4B Q8_0)", Endpoint: "http://127.0.0.1:8080/v1", APIKey: "sk-local"},
}

func main() {
	flag.Parse()

	printHeader()

	if *flagEndpoint != "" {
		benchEndpoint(*flagEndpoint, "sk-local", "custom")
		return
	}

	if *flagAll {
		for _, p := range defaultProfiles {
			benchEndpoint(p.Endpoint, p.APIKey, p.Name)
			fmt.Println()
		}
		return
	}

	// Single profile
	for _, p := range defaultProfiles {
		if strings.Contains(strings.ToLower(p.Name), *flagModel) {
			benchEndpoint(p.Endpoint, p.APIKey, p.Name)
			return
		}
	}

	// Fallback to first reachable
	for _, p := range defaultProfiles {
		if isReachable(p.Endpoint) {
			benchEndpoint(p.Endpoint, p.APIKey, p.Name)
			return
		}
	}

	fmt.Printf("%sNo reachable endpoints found. Start a llama-server first.%s\n", red, reset)
	os.Exit(1)
}

func printHeader() {
	fmt.Printf("%s════════════════════════════════════════════════════%s\n", yellow, reset)
	fmt.Printf(" Gilgamesh Benchmark Suite\n")

	// System info
	if data, err := exec.Command("lscpu").Output(); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "Model name") {
				cpu := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				fmt.Printf(" CPU: %s\n", cpu)
			}
		}
	}
	if data, err := exec.Command("free", "-h").Output(); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "Mem:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					fmt.Printf(" RAM: %s\n", fields[1])
				}
			}
		}
	}
	fmt.Printf("%s════════════════════════════════════════════════════%s\n\n", yellow, reset)
}

func benchEndpoint(endpoint, apiKey, label string) {
	fmt.Printf("%s── %s ──%s\n", cyan, label, reset)
	fmt.Printf("   endpoint: %s\n", endpoint)

	if !isReachable(endpoint) {
		fmt.Printf("   %sSKIP: endpoint not reachable%s\n", red, reset)
		return
	}

	// 1. Health check latency
	benchHealth(endpoint)

	// 2. Minimal prompt (non-streaming) — measures TTFT + generation
	benchMinimalPrompt(endpoint, apiKey)

	// 3. Tool-calling prompt — measures tool call parsing
	benchToolCall(endpoint, apiKey)

	// 4. Gilgamesh one-shot — end-to-end agent benchmark
	benchGilgameshOneShot(endpoint, apiKey)

	// 5. Gilgamesh edit task — tests tool loop quality
	benchGilgameshEdit(endpoint, apiKey)
}

func benchHealth(endpoint string) {
	baseURL := strings.TrimSuffix(endpoint, "/v1")
	start := time.Now()
	resp, err := http.Get(baseURL + "/health")
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("   health:   %s%v%s\n", red, err, reset)
		return
	}
	resp.Body.Close()
	fmt.Printf("   health:   %s%s%s (%d)\n", green, elapsed.Round(time.Millisecond), reset, resp.StatusCode)
}

func benchMinimalPrompt(endpoint, apiKey string) {
	body := `{
		"model": "bench",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "What is 2+2? One sentence."}
		],
		"max_tokens": 50
	}`

	start := time.Now()
	data, err := apiCall(endpoint, apiKey, body)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("   minimal:  %s%v%s\n", red, err, reset)
		return
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	json.Unmarshal(data, &resp)

	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
		if len(content) > 80 {
			content = content[:80]
		}
		content = strings.ReplaceAll(content, "\n", " ")
	}

	ctTok := float64(resp.Usage.CompletionTokens)
	elMs := float64(elapsed.Milliseconds())
	tokPerSec := 0.0
	if elMs > 0 && ctTok > 0 {
		tokPerSec = ctTok / (elMs / 1000.0)
	}

	fmt.Printf("   minimal:  %s%s%s | pp=%d ct=%d (%.1f tok/s) | %s\n",
		green, elapsed.Round(time.Millisecond), reset,
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, tokPerSec, content)
}

func benchToolCall(endpoint, apiKey string) {
	body := `{
		"model": "bench",
		"messages": [
			{"role": "system", "content": "You are a coding assistant. Use the provided tools."},
			{"role": "user", "content": "List all Go files in the current directory."}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "glob",
				"description": "Find files matching a pattern",
				"parameters": {"type": "object", "properties": {"pattern": {"type": "string"}}, "required": ["pattern"]}
			}
		}],
		"max_tokens": 200
	}`

	start := time.Now()
	data, err := apiCall(endpoint, apiKey, body)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("   toolcall: %s%v%s\n", red, err, reset)
		return
	}

	var resp struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(data, &resp)

	toolCalls := 0
	toolName := ""
	if len(resp.Choices) > 0 {
		toolCalls = len(resp.Choices[0].Message.ToolCalls)
		if toolCalls > 0 {
			toolName = resp.Choices[0].Message.ToolCalls[0].Function.Name
		}
	}

	if toolCalls > 0 {
		fmt.Printf("   toolcall: %s%s%s | %stool=%s%s (%d calls)\n",
			green, elapsed.Round(time.Millisecond), reset, green, toolName, reset, toolCalls)
	} else {
		fmt.Printf("   toolcall: %s%s%s | %sno tool calls (text only)%s\n",
			yellow, elapsed.Round(time.Millisecond), reset, yellow, reset)
	}
}

func benchGilgameshOneShot(endpoint, apiKey string) {
	gilgamesh := findGilgamesh()
	if gilgamesh == "" {
		fmt.Printf("   oneshot:  %sSKIP (gilgamesh binary not found)%s\n", dim, reset)
		return
	}

	// Create temp config
	cfg := fmt.Sprintf(`{"models":{"bench":{"name":"bench","endpoint":"%s","api_key":"%s"}},"active_model":"bench"}`, endpoint, apiKey)
	tmpCfg, _ := os.CreateTemp("", "gilgamesh-bench-*.json")
	tmpCfg.WriteString(cfg)
	tmpCfg.Close()
	defer os.Remove(tmpCfg.Name())

	// Run from a temp directory with the config
	tmpDir, _ := os.MkdirTemp("", "gilgamesh-bench-*")
	defer os.RemoveAll(tmpDir)
	os.WriteFile(tmpDir+"/gilgamesh.json", []byte(cfg), 0644)

	cmd := exec.Command(gilgamesh, "run", "What is 2+2? Reply in one sentence.")
	cmd.Dir = tmpDir

	start := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("   oneshot:  %s%s%s | %serror: %v%s\n", yellow, elapsed.Round(time.Millisecond), reset, red, err, reset)
		if *flagVerbose {
			fmt.Printf("            %s%s%s\n", dim, truncate(string(output), 200), reset)
		}
		return
	}

	response := strings.TrimSpace(string(output))
	lines := strings.Split(response, "\n")
	// Get last non-empty line (the actual response)
	var lastLine string
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" {
			lastLine = l
			break
		}
	}
	if len(lastLine) > 100 {
		lastLine = lastLine[:100]
	}

	fmt.Printf("   oneshot:  %s%s%s | %s\n", green, elapsed.Round(time.Millisecond), reset, lastLine)
}

func benchGilgameshEdit(endpoint, apiKey string) {
	gilgamesh := findGilgamesh()
	if gilgamesh == "" {
		fmt.Printf("   edit:     %sSKIP (gilgamesh binary not found)%s\n", dim, reset)
		return
	}

	cfg := fmt.Sprintf(`{"models":{"bench":{"name":"bench","endpoint":"%s","api_key":"%s"}},"active_model":"bench"}`, endpoint, apiKey)
	tmpDir, _ := os.MkdirTemp("", "gilgamesh-bench-edit-*")
	defer os.RemoveAll(tmpDir)
	os.WriteFile(tmpDir+"/gilgamesh.json", []byte(cfg), 0644)

	testFile := tmpDir + "/hello.py"

	cmd := exec.Command(gilgamesh, "run",
		fmt.Sprintf("Create %s that prints 'hello'. Then edit it to print 'hello world' instead.", testFile))
	cmd.Dir = tmpDir

	start := time.Now()
	output, _ := cmd.CombinedOutput()
	elapsed := time.Since(start)

	// Check result
	result := "FAIL"
	if data, err := os.ReadFile(testFile); err == nil {
		if strings.Contains(string(data), "hello world") {
			result = green + "PASS" + reset
		}
	}

	toolCalls := strings.Count(string(output), "⚡")
	fmt.Printf("   edit:     %s%s%s | tools=%d | %s\n", green, elapsed.Round(time.Millisecond), reset, toolCalls, result)
}

// --- helpers ---

func isReachable(endpoint string) bool {
	baseURL := strings.TrimSuffix(endpoint, "/v1")
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func apiCall(endpoint, apiKey, body string) ([]byte, error) {
	url := strings.TrimSuffix(endpoint, "/") + "/chat/completions"
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func findGilgamesh() string {
	// Check common locations
	paths := []string{
		"./gilgamesh",
		"../gilgamesh",
		"/root/godsfromthemachine/gilgamesh/gilgamesh",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// Try PATH
	if p, err := exec.LookPath("gilgamesh"); err == nil {
		return p
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
