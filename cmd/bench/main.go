// cmd/bench benchmarks gilgamesh against local LLM endpoints.
//
// Loads model profiles from gilgamesh.json config. Runs up to 6 benchmarks
// per profile: health, raw inference (llama-bench), minimal prompt, tool call,
// one-shot agent, and edit task.
//
// Usage:
//
//	go run ./cmd/bench                     # bench active profile from config
//	go run ./cmd/bench -all                # bench all configured profiles
//	go run ./cmd/bench -model heavy        # bench specific profile
//	go run ./cmd/bench -endpoint URL       # bench custom endpoint
//	go run ./cmd/bench -raw                # include raw llama-bench metrics
//	go run ./cmd/bench -json               # output JSON results
//	go run ./cmd/bench -save results.json  # append results to JSON log
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	flagEndpoint = flag.String("endpoint", "", "LLM endpoint URL (overrides config)")
	flagModel    = flag.String("model", "", "Model profile to benchmark")
	flagAll      = flag.Bool("all", false, "Benchmark all configured profiles")
	flagRaw      = flag.Bool("raw", false, "Include raw llama-bench inference speed")
	flagJSON     = flag.Bool("json", false, "Output results as JSON")
	flagSave     = flag.String("save", "", "Append JSON results to file")
	flagVerbose  = flag.Bool("v", false, "Verbose output")
)

// ANSI colors
var (
	cCyan   = "\033[36m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cRed    = "\033[31m"
	cDim    = "\033[2m"
	cReset  = "\033[0m"
)

func disableColors() {
	cCyan, cGreen, cYellow, cRed, cDim, cReset = "", "", "", "", "", ""
}

// --- Result types ---

type BenchResult struct {
	Timestamp string      `json:"timestamp"`
	System    SystemInfo  `json:"system"`
	Profile   string      `json:"profile"`
	Endpoint  string      `json:"endpoint"`
	Health    *HealthR    `json:"health,omitempty"`
	Raw       *RawR       `json:"raw,omitempty"`
	Minimal   *PromptR    `json:"minimal,omitempty"`
	ToolCall  *ToolCallR  `json:"tool_call,omitempty"`
	OneShot   *OneShotR   `json:"one_shot,omitempty"`
	Edit      *EditR      `json:"edit,omitempty"`
}

type SystemInfo struct {
	CPU   string `json:"cpu"`
	Cores int    `json:"cores"`
	RAM   string `json:"ram"`
}

type HealthR struct {
	LatencyMs int `json:"latency_ms"`
	Status    int `json:"status"`
}

type RawR struct {
	PP      float64 `json:"pp_tok_s"`
	TG      float64 `json:"tg_tok_s"`
	Threads int     `json:"threads"`
}

type PromptR struct {
	ElapsedMs        int     `json:"elapsed_ms"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TokPerSec        float64 `json:"tok_per_sec"`
	Content          string  `json:"content"`
}

type ToolCallR struct {
	ElapsedMs int    `json:"elapsed_ms"`
	ToolName  string `json:"tool_name"`
	ToolCalls int    `json:"tool_calls"`
}

type OneShotR struct {
	ElapsedMs int    `json:"elapsed_ms"`
	Response  string `json:"response"`
}

type EditR struct {
	ElapsedMs int  `json:"elapsed_ms"`
	ToolCalls int  `json:"tool_calls"`
	Pass      bool `json:"pass"`
}

// --- Config (mirrors config package for standalone binary) ---

type modelCfg struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key"`
}

type benchCfg struct {
	Models      map[string]modelCfg `json:"models"`
	ActiveModel string              `json:"active_model"`
}

func loadConfig() *benchCfg {
	cfg := &benchCfg{
		Models: map[string]modelCfg{
			"fast":    {Name: "qwen3.5-2b", Endpoint: "http://127.0.0.1:8081/v1", APIKey: "sk-local"},
			"default": {Name: "qwen3.5-2b", Endpoint: "http://127.0.0.1:8081/v1", APIKey: "sk-local"},
			"heavy":   {Name: "qwen3.5-4b", Endpoint: "http://127.0.0.1:8080/v1", APIKey: "sk-local"},
		},
		ActiveModel: "default",
	}

	paths := []string{"gilgamesh.json"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "gilgamesh", "gilgamesh.json"))
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		json.Unmarshal(data, cfg)
		break
	}
	return cfg
}

// --- System info ---

func collectSystemInfo() SystemInfo {
	info := SystemInfo{Cores: runtime.NumCPU()}

	if data, err := exec.Command("lscpu").Output(); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "Model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.CPU = strings.TrimSpace(parts[1])
				}
			}
		}
	}
	if data, err := exec.Command("free", "-h").Output(); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "Mem:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					info.RAM = fields[1]
				}
			}
		}
	}
	return info
}

// --- Main ---

func main() {
	flag.Parse()

	if *flagJSON {
		disableColors()
	}

	cfg := loadConfig()
	sysInfo := collectSystemInfo()
	var results []BenchResult

	if !*flagJSON {
		printHeader(sysInfo, cfg)
	}

	if *flagEndpoint != "" {
		r := benchEndpoint(*flagEndpoint, "sk-local", "custom", "", sysInfo)
		results = append(results, r)
	} else if *flagAll {
		// Sort profile names for consistent ordering
		names := make([]string, 0, len(cfg.Models))
		for name := range cfg.Models {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			m := cfg.Models[name]
			r := benchEndpoint(m.Endpoint, m.APIKey, name, m.Name, sysInfo)
			results = append(results, r)
			if !*flagJSON {
				fmt.Println()
			}
		}
	} else {
		profileName := cfg.ActiveModel
		if *flagModel != "" {
			profileName = *flagModel
		}
		m, ok := cfg.Models[profileName]
		if !ok {
			fmt.Fprintf(os.Stderr, "%sProfile %q not found. Available:", cRed, profileName)
			for name := range cfg.Models {
				fmt.Fprintf(os.Stderr, " %s", name)
			}
			fmt.Fprintf(os.Stderr, "%s\n", cReset)
			os.Exit(1)
		}
		r := benchEndpoint(m.Endpoint, m.APIKey, profileName, m.Name, sysInfo)
		results = append(results, r)
	}

	// Output
	if *flagJSON {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(data))
	} else if len(results) > 1 {
		printSummary(results)
	}

	if *flagSave != "" {
		saveResults(*flagSave, results)
	}
}

func printHeader(info SystemInfo, cfg *benchCfg) {
	fmt.Printf("%s════════════════════════════════════════════════════%s\n", cYellow, cReset)
	fmt.Printf(" Gilgamesh Benchmark Suite\n")
	if info.CPU != "" {
		fmt.Printf(" CPU: %s (%d cores)\n", info.CPU, info.Cores)
	}
	if info.RAM != "" {
		fmt.Printf(" RAM: %s\n", info.RAM)
	}
	fmt.Printf(" Profiles: ")
	for name, m := range cfg.Models {
		fmt.Printf("%s(%s) ", name, m.Name)
	}
	fmt.Println()
	fmt.Printf("%s════════════════════════════════════════════════════%s\n\n", cYellow, cReset)
}

// --- Benchmark orchestration ---

func benchEndpoint(endpoint, apiKey, profileName, modelName string, sysInfo SystemInfo) BenchResult {
	result := BenchResult{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		System:    sysInfo,
		Profile:   profileName,
		Endpoint:  endpoint,
	}

	if !*flagJSON {
		label := profileName
		if modelName != "" {
			label = profileName + " (" + modelName + ")"
		}
		fmt.Printf("%s── %s ──%s\n", cCyan, label, cReset)
		fmt.Printf("   endpoint: %s\n", endpoint)
	}

	if !isReachable(endpoint) {
		if !*flagJSON {
			fmt.Printf("   %sSKIP: endpoint not reachable%s\n", cRed, cReset)
		}
		return result
	}

	// 1. Health
	result.Health = benchHealth(endpoint)

	// 2. Raw llama-bench (optional)
	if *flagRaw {
		result.Raw = benchRaw(modelName)
	}

	// 3. Minimal prompt
	result.Minimal = benchMinimalPrompt(endpoint, apiKey)

	// 4. Tool call
	result.ToolCall = benchToolCall(endpoint, apiKey)

	// 5. One-shot agent
	result.OneShot = benchGilgameshOneShot(endpoint, apiKey)

	// 6. Edit task
	result.Edit = benchGilgameshEdit(endpoint, apiKey)

	return result
}

// --- Individual benchmarks ---

func benchHealth(endpoint string) *HealthR {
	baseURL := strings.TrimSuffix(endpoint, "/v1")
	start := time.Now()
	resp, err := http.Get(baseURL + "/health")
	elapsed := time.Since(start)

	if err != nil {
		if !*flagJSON {
			fmt.Printf("   health:   %s%v%s\n", cRed, err, cReset)
		}
		return nil
	}
	resp.Body.Close()

	r := &HealthR{
		LatencyMs: int(elapsed.Milliseconds()),
		Status:    resp.StatusCode,
	}

	if !*flagJSON {
		fmt.Printf("   health:   %s%dms%s (status %d)\n", cGreen, r.LatencyMs, cReset, r.Status)
	}
	return r
}

func benchRaw(modelName string) *RawR {
	llamaBench := findLlamaBench()
	if llamaBench == "" {
		if !*flagJSON {
			fmt.Printf("   raw:      %sSKIP (llama-bench not found)%s\n", cDim, cReset)
		}
		return nil
	}

	modelPath := findModelFile(modelName)
	if modelPath == "" {
		if !*flagJSON {
			fmt.Printf("   raw:      %sSKIP (model file not found for %q)%s\n", cDim, modelName, cReset)
		}
		return nil
	}

	threads := runtime.NumCPU()
	cmd := exec.Command(llamaBench,
		"-m", modelPath,
		"-p", "512", "-n", "32",
		"-t", strconv.Itoa(threads),
		"-r", "1",
	)

	if !*flagJSON {
		fmt.Printf("   raw:      running llama-bench (pp512/tg32, %d threads)...\n", threads)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if !*flagJSON {
			fmt.Printf("   raw:      %serror: %v%s\n", cRed, err, cReset)
			if *flagVerbose {
				fmt.Printf("            %s%s%s\n", cDim, truncate(string(output), 200), cReset)
			}
		}
		return nil
	}

	r := &RawR{Threads: threads}

	// Parse llama-bench markdown table output
	// Format: | model | size | params | backend | threads | test | t/s |
	// The t/s column contains "170.10 ± 0.00" — we extract the value before ±
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(line, "|")
		if len(fields) < 7 {
			continue
		}

		// fields[0]="" fields[1]=model ... fields[6]=test fields[7]=t/s fields[8]=""
		testField := strings.TrimSpace(fields[len(fields)-3])
		valueField := strings.TrimSpace(fields[len(fields)-2])

		// Extract number before "±" (e.g. "170.10 ± 0.00" → "170.10")
		if idx := strings.Index(valueField, "±"); idx > 0 {
			valueField = strings.TrimSpace(valueField[:idx])
		}

		tokPerSec, err := strconv.ParseFloat(valueField, 64)
		if err != nil {
			continue
		}

		if strings.HasPrefix(testField, "pp") {
			r.PP = tokPerSec
		} else if strings.HasPrefix(testField, "tg") {
			r.TG = tokPerSec
		}
	}

	if r.PP > 0 || r.TG > 0 {
		if !*flagJSON {
			fmt.Printf("   raw:      %spp=%.0f tok/s  tg=%.1f tok/s%s (%d threads)\n",
				cGreen, r.PP, r.TG, cReset, threads)
		}
		return r
	}

	// Fallback: try to extract from non-table output
	if *flagVerbose && !*flagJSON {
		fmt.Printf("   raw:      %scould not parse output%s\n", cYellow, cReset)
		for _, line := range strings.Split(string(output), "\n") {
			if strings.TrimSpace(line) != "" {
				fmt.Printf("            %s%s%s\n", cDim, line, cReset)
			}
		}
	}
	return nil
}

func benchMinimalPrompt(endpoint, apiKey string) *PromptR {
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
		if !*flagJSON {
			fmt.Printf("   minimal:  %s%v%s\n", cRed, err, cReset)
		}
		return nil
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
		content = strings.ReplaceAll(content, "\n", " ")
	}

	elMs := elapsed.Milliseconds()
	ctTok := float64(resp.Usage.CompletionTokens)
	tokPerSec := 0.0
	if elMs > 0 && ctTok > 0 {
		tokPerSec = ctTok / (float64(elMs) / 1000.0)
	}

	r := &PromptR{
		ElapsedMs:        int(elMs),
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TokPerSec:        tokPerSec,
		Content:          truncate(content, 80),
	}

	if !*flagJSON {
		fmt.Printf("   minimal:  %s%dms%s | pp=%d ct=%d (%.1f tok/s) | %s\n",
			cGreen, r.ElapsedMs, cReset,
			r.PromptTokens, r.CompletionTokens, r.TokPerSec,
			truncate(content, 60))
	}
	return r
}

func benchToolCall(endpoint, apiKey string) *ToolCallR {
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
		if !*flagJSON {
			fmt.Printf("   toolcall: %s%v%s\n", cRed, err, cReset)
		}
		return nil
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

	r := &ToolCallR{ElapsedMs: int(elapsed.Milliseconds())}
	if len(resp.Choices) > 0 {
		r.ToolCalls = len(resp.Choices[0].Message.ToolCalls)
		if r.ToolCalls > 0 {
			r.ToolName = resp.Choices[0].Message.ToolCalls[0].Function.Name
		}
	}

	if !*flagJSON {
		if r.ToolCalls > 0 {
			fmt.Printf("   toolcall: %s%dms%s | %stool=%s%s (%d calls)\n",
				cGreen, r.ElapsedMs, cReset, cGreen, r.ToolName, cReset, r.ToolCalls)
		} else {
			fmt.Printf("   toolcall: %s%dms%s | %sno tool calls (text only)%s\n",
				cYellow, r.ElapsedMs, cReset, cYellow, cReset)
		}
	}
	return r
}

func benchGilgameshOneShot(endpoint, apiKey string) *OneShotR {
	gilgamesh := findGilgamesh()
	if gilgamesh == "" {
		if !*flagJSON {
			fmt.Printf("   oneshot:  %sSKIP (gilgamesh binary not found — run go build first)%s\n", cDim, cReset)
		}
		return nil
	}

	cfg := fmt.Sprintf(`{"models":{"bench":{"name":"bench","endpoint":"%s","api_key":"%s"}},"active_model":"bench"}`, endpoint, apiKey)
	tmpDir, _ := os.MkdirTemp("", "gilgamesh-bench-*")
	defer os.RemoveAll(tmpDir)
	os.WriteFile(tmpDir+"/gilgamesh.json", []byte(cfg), 0644)

	cmd := exec.Command(gilgamesh, "run", "What is 2+2? Reply in one sentence.")
	cmd.Dir = tmpDir

	start := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	r := &OneShotR{ElapsedMs: int(elapsed.Milliseconds())}

	if err != nil {
		if !*flagJSON {
			fmt.Printf("   oneshot:  %s%dms%s | %serror: %v%s\n",
				cYellow, r.ElapsedMs, cReset, cRed, err, cReset)
			if *flagVerbose {
				fmt.Printf("            %s%s%s\n", cDim, truncate(string(output), 200), cReset)
			}
		}
		return r
	}

	// Extract last meaningful line as response, skipping timing and startup info
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(stripAnsi(lines[i]))
		if l == "" || strings.HasPrefix(l, "(") || strings.HasPrefix(l, "gilgamesh") {
			continue
		}
		r.Response = truncate(l, 100)
		break
	}

	if !*flagJSON {
		fmt.Printf("   oneshot:  %s%dms%s | %s\n", cGreen, r.ElapsedMs, cReset, r.Response)
	}
	return r
}

func benchGilgameshEdit(endpoint, apiKey string) *EditR {
	gilgamesh := findGilgamesh()
	if gilgamesh == "" {
		if !*flagJSON {
			fmt.Printf("   edit:     %sSKIP (gilgamesh binary not found)%s\n", cDim, cReset)
		}
		return nil
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

	r := &EditR{ElapsedMs: int(elapsed.Milliseconds())}

	// Check result
	if data, err := os.ReadFile(testFile); err == nil {
		if strings.Contains(string(data), "hello world") {
			r.Pass = true
		}
	}

	r.ToolCalls = strings.Count(string(output), "⚡")

	if !*flagJSON {
		passStr := cRed + "FAIL" + cReset
		if r.Pass {
			passStr = cGreen + "PASS" + cReset
		}
		fmt.Printf("   edit:     %s%dms%s | tools=%d | %s\n",
			cGreen, r.ElapsedMs, cReset, r.ToolCalls, passStr)
	}
	return r
}

// --- Output helpers ---

func printSummary(results []BenchResult) {
	fmt.Printf("\n%s════════════════════════════════════════════════════%s\n", cYellow, cReset)
	fmt.Printf(" Summary\n")
	fmt.Printf("%s════════════════════════════════════════════════════%s\n", cYellow, cReset)
	fmt.Printf(" %-18s %8s %8s %8s %8s %5s\n", "Profile", "Health", "Minimal", "ToolCall", "OneShot", "Edit")
	fmt.Printf(" %-18s %8s %8s %8s %8s %5s\n", "───────", "──────", "───────", "────────", "───────", "────")

	for _, r := range results {
		health := "—"
		if r.Health != nil {
			health = fmt.Sprintf("%dms", r.Health.LatencyMs)
		}
		minimal := "—"
		if r.Minimal != nil {
			minimal = fmt.Sprintf("%dms", r.Minimal.ElapsedMs)
		}
		toolcall := "—"
		if r.ToolCall != nil {
			toolcall = fmt.Sprintf("%dms", r.ToolCall.ElapsedMs)
		}
		oneshot := "—"
		if r.OneShot != nil {
			oneshot = fmt.Sprintf("%dms", r.OneShot.ElapsedMs)
		}
		edit := "—"
		if r.Edit != nil {
			if r.Edit.Pass {
				edit = cGreen + "PASS" + cReset
			} else {
				edit = cRed + "FAIL" + cReset
			}
		}
		fmt.Printf(" %-18s %8s %8s %8s %8s %5s\n",
			truncate(r.Profile, 18), health, minimal, toolcall, oneshot, edit)
	}
	fmt.Println()
}

func saveResults(path string, results []BenchResult) {
	// Load existing results
	var all []BenchResult
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &all)
	}

	all = append(all, results...)

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "save error: %v\n", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "save error: %v\n", err)
		return
	}

	if !*flagJSON {
		fmt.Printf("%sSaved %d result(s) to %s (%d total)%s\n",
			cDim, len(results), path, len(all), cReset)
	}
}

// --- Finders ---

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
	paths := []string{
		"./gilgamesh",
		"../gilgamesh",
		"/root/godsfromthemachine/gilgamesh/gilgamesh",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			abs, err := filepath.Abs(p)
			if err == nil {
				return abs
			}
			return p
		}
	}
	if p, err := exec.LookPath("gilgamesh"); err == nil {
		return p
	}
	return ""
}

func findLlamaBench() string {
	// Check env var first
	if p := os.Getenv("LLAMA_BENCH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	paths := []string{
		"./local-ai/bin/llama-bench",
		"../local-ai/bin/llama-bench",
		"/root/godsfromthemachine/gilgamesh/local-ai/bin/llama-bench",
		"/root/battlestation/local-ai/llama.cpp/build/bin/llama-bench",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("llama-bench"); err == nil {
		return p
	}
	return ""
}

func findModelFile(modelName string) string {
	// Check env var for model directory
	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		// Try common locations
		for _, dir := range []string{
			"./local-ai/models",
			"../local-ai/models",
			"/root/godsfromthemachine/gilgamesh/local-ai/models",
			"/root/battlestation/local-ai/models",
		} {
			if _, err := os.Stat(dir); err == nil {
				modelDir = dir
				break
			}
		}
	}
	if modelDir == "" {
		return ""
	}

	// Map model names to file patterns
	// Config names like "qwen3.5-2b" → directory "Qwen3.5-2B"
	nameUpper := strings.ReplaceAll(strings.ToUpper(modelName), "QWEN", "Qwen")
	nameUpper = strings.ReplaceAll(nameUpper, "qwen", "Qwen")

	// Try to match model name to directory
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		dirName := entry.Name()
		// Resolve symlinks — os.ReadDir reports symlinks as non-directories
		subDir := filepath.Join(modelDir, dirName)
		fi, err := os.Stat(subDir)
		if err != nil || !fi.IsDir() {
			continue
		}
		// Check if directory name contains the model name (case-insensitive)
		if !strings.Contains(strings.ToLower(dirName), strings.ToLower(strings.ReplaceAll(modelName, ".", ""))) &&
			!strings.Contains(strings.ToLower(dirName), strings.ToLower(modelName)) {
			continue
		}

		// Find a .gguf file in this directory, preferring Q4_K_M for speed
		ggufFiles, _ := filepath.Glob(filepath.Join(subDir, "*.gguf"))
		if len(ggufFiles) == 0 {
			continue
		}

		// Prefer Q4_K_M quant for benchmarks (faster)
		for _, f := range ggufFiles {
			if strings.Contains(f, "Q4_K_M") {
				return f
			}
		}
		return ggufFiles[0]
	}

	// Direct name match attempt
	_ = nameUpper
	return ""
}

func stripAnsi(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until 'm'
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
