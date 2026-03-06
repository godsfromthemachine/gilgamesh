# Agents Guide — Gilgamesh

## Project Overview

Gilgamesh is a TDD-driven local AI coding agent written in Go. It connects to a local llama.cpp server (or any OpenAI-compatible endpoint) and provides tool-calling capabilities for software engineering tasks. Module: `github.com/godsfromthemachine/gilgamesh`. Go 1.25+. Zero external dependencies — Go stdlib only.

## Architecture

```
gilgamesh/
├── main.go              CLI entry, REPL, subcommand dispatch
├── agent/
│   ├── agent.go         Core agent loop (Run for CLI, RunWithEvents for HTTP)
│   └── prompt.go        TDD-first system prompt (~300 tokens)
├── llm/
│   └── client.go        OpenAI-compatible streaming SSE client
├── tools/
│   ├── registry.go      Tool registration, dispatch, enumeration
│   ├── read.go          Read files (offset/limit, numbered lines, 500 line cap)
│   ├── write.go         Create/overwrite files (auto-mkdir)
│   ├── edit.go          Find-and-replace (unique match required)
│   ├── bash.go          Shell execution (120s timeout, 10KB cap)
│   ├── grep.go          Regex content search (50 match cap)
│   ├── glob.go          File pattern matching (100 file cap)
│   └── test.go          Go test runner (300s timeout, 15KB cap)
├── mcp/
│   ├── protocol.go      JSON-RPC 2.0 + MCP protocol types
│   └── server.go        MCP stdio server (initialize, tools/list, tools/call)
├── server/
│   └── server.go        HTTP API server (health, tools, chat SSE)
├── cmd/bench/
│   └── main.go          Model benchmark tool (health, prompt, toolcall, oneshot, edit)
├── config/
│   └── config.go        JSON config loader (fast/default/heavy model profiles)
├── context/
│   └── context.go       Project context (.gilgameshfile) + skills loader
├── hooks/
│   └── hooks.go         Pre/post tool execution hooks (.gilgamesh/hooks.json)
└── session/
    └── session.go       JSONL session logging + distill summaries
```

## Three Interfaces (CLI / MCP / API)

All three interfaces share the same `tools.Registry`. No capability is exclusive to any interface.

- **CLI**: `./gilgamesh` (interactive REPL), `./gilgamesh run "prompt"` (one-shot)
- **MCP**: `./gilgamesh mcp` — JSON-RPC 2.0 over stdio for Claude Desktop and other agents
- **HTTP**: `./gilgamesh serve [-p PORT]` — REST + SSE at `/api/health`, `/api/tools`, `/api/tools/{name}`, `/api/chat`

## Build, Test, Run

```bash
go build -o gilgamesh .          # build
go test ./... -v -cover          # test all packages
./gilgamesh                      # interactive REPL
./gilgamesh run "prompt"         # one-shot
./gilgamesh mcp                  # MCP server
./gilgamesh serve -p 7777        # HTTP API server
./gilgamesh -m heavy run "task"  # use heavy model profile
```

## Key Conventions

### Code Style
- Go stdlib only. No third-party packages. Do not add any.
- Concise error handling. Short variable names. Minimal imports.
- ANSI color codes for terminal output (defined in main.go).
- Each tool in its own file under `tools/`. Function returns `*Tool` struct.
- `Parameters` field is JSON Schema as `json.RawMessage`.
- `Execute` is a closure: `func(args json.RawMessage) (string, error)`.

### Token Budget
This project is designed for CPU inference with small models (2B-4B parameters). Token overhead is critical:
- System prompt: ~300 tokens
- 7 tool definitions: ~800 tokens
- Project context: ~500 tokens (capped)
- Total overhead: ~1,600 tokens

Do NOT bloat the system prompt, tool descriptions, or parameter schemas. Keep them minimal.

### Tool Output Caps
- read: 500 lines max
- bash: 10KB (first 5K + last 2K if truncated)
- grep: 50 matches, 8KB
- test: 15KB (first 7K + last 3K)
- Session log entries: tool results truncated to 500 chars

### Agent Loop
- Max 15 tool call iterations per user message
- Loop detection: same tool+args repeated 2x triggers forced response
- Context compaction at 12K tokens — old tool results trimmed to 3 lines
- Hooks run before (can block) and after (observe) tool execution
- 10-second timeout per hook

### Testing
- Tests use Go's standard `testing` package only
- Table-driven tests preferred
- Test files: `tools/registry_test.go`, `mcp/server_test.go`
- Helper functions in test files (e.g., `containsStr`, `itoa`, `rpc`)
- Run with `go test ./... -v -cover`

### Skills
- Located in `.gilgamesh/skills/*.md` (project-local) or `~/.config/gilgamesh/skills/`
- Format: first line is description (after `#`), rest is prompt template
- `{{args}}` placeholder replaced with invocation arguments
- Invoked via `/skillname` or `/skillname args` in the REPL

### Configuration
- `gilgamesh.json` in project root or `~/.config/gilgamesh/gilgamesh.json`
- Model profiles: `fast`, `default`, `heavy` with name, endpoint, api_key
- Default endpoints: fast/default → `:8081`, heavy → `:8080`

### Version
- Version hardcoded as `version` constant in `main.go`
- Currently `0.3.0`

## Benchmarking & Model Trials

See `TRIALS.md` for detailed model benchmarking results and findings. The benchmark tool lives at `cmd/bench/main.go`:

```bash
go run ./cmd/bench              # bench default endpoint
go run ./cmd/bench -all         # bench all profiles
go run ./cmd/bench -model heavy # bench specific profile
```

Measures: health latency, minimal prompt (TTFT + tok/s), tool call parsing, gilgamesh one-shot, and edit task (create + modify file).

## Adding a New Tool

1. Create `tools/newtool.go` with a function returning `*Tool`
2. Define JSON Schema parameters as `json.RawMessage`
3. Implement `Execute` closure that parses args and returns `(string, error)`
4. Register in `tools/registry.go` `NewRegistry()` function
5. Add tests in `tools/registry_test.go` or a new test file
6. Update tool count assertions in tests (currently expect 7)
7. Keep description and schema lean — every token costs

## Adding a New Subcommand

Subcommands are parsed in `main.go` `main()` function via simple string matching on `os.Args`. Add a new case before the default REPL fallthrough.
