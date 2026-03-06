# CLAUDE.md — Gilgamesh

## What is this?

Gilgamesh is a TDD-driven local AI coding agent. Go module `github.com/godsfromthemachine/gilgamesh`, Go 1.25+, zero external dependencies. Part of [Gods from the Machine](https://github.com/godsfromthemachine).

## Build & Test

```bash
go build -o gilgamesh .      # build binary
go test ./... -v -cover      # run all tests
```

## Run

```bash
./gilgamesh                      # interactive REPL
./gilgamesh run "prompt"         # one-shot
./gilgamesh mcp                  # MCP server (JSON-RPC 2.0 stdio)
./gilgamesh serve -p 7777        # HTTP API server
./gilgamesh -m heavy run "task"  # use heavy model
```

## Critical Rules

- **Zero external deps.** Go stdlib only. Never add third-party packages.
- **Token budget matters.** System prompt ~300 tokens, tools ~800, context ~500. Total ~1,600. Do not bloat prompts, descriptions, or schemas.
- **Keep tool output capped.** read: 500 lines, bash: 10KB, grep: 50 matches, test: 15KB.
- **Test everything.** This is a TDD agent — it must have comprehensive tests. Use table-driven tests with `testing` stdlib.
- **Each tool is a file.** `tools/newtool.go` returns `*Tool` with `Name`, `Description`, `Parameters` (JSON Schema), `Execute` (closure).
- **All three interfaces share one registry.** CLI, MCP, and HTTP all use `tools.Registry`. No capability is exclusive to any interface.
- **Version is in `main.go`.** Constant `version = "0.3.0"`.

## Project Structure

```
main.go           CLI entry, REPL, subcommand dispatch
agent/agent.go    Core loop: prompt → LLM → tool calls → repeat (max 15 loops)
agent/prompt.go   System prompt (~300 tokens, TDD-first)
llm/client.go     OpenAI-compatible streaming SSE client
tools/registry.go Tool registration + dispatch; tools/*.go = 7 built-in tools
mcp/protocol.go   JSON-RPC 2.0 types; mcp/server.go = MCP stdio server
server/server.go  HTTP API: /api/health, /api/tools, /api/tools/{name}, /api/chat (SSE)
config/config.go  Model profiles (fast/default/heavy)
context/context.go Project context + skills (.gilgamesh/skills/*.md)
hooks/hooks.go    Pre/post tool hooks (.gilgamesh/hooks.json)
session/session.go JSONL session logging
```

## Benchmarking

```bash
go run ./cmd/bench              # bench default endpoint
go run ./cmd/bench -all         # bench all profiles
go run ./cmd/bench -model heavy # bench specific profile
```

See `TRIALS.md` for model trialing results and findings.

## Key Behaviors

- Loop detection: same tool+args 2x → forced response
- Context compaction at ~12K tokens — old tool results trimmed
- Pre-hooks can block tool execution; post-hooks observe only
- Hook timeout: 10s. Bash timeout: 120s. Test timeout: 300s.
- Skills use `{{args}}` placeholder, injected as user message

## Commit Conventions

- Author: `bkataru <baalateja.k@gmail.com>`
- Keep commits focused and descriptive
- Run `go build -o gilgamesh . && go test ./... -v` before committing
