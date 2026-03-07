# Context Guide

How to make gilgamesh aware of your project and environment without bloating the token budget.

## Project Context

Gilgamesh loads project context from one of these files (first found wins):

1. `.gilgameshfile` — flat text file in project root
2. `.gilgamesh/context.md` — markdown file in config directory

The content is injected into the system prompt and capped at ~500 tokens (~2000 characters). Keep it concise — every token counts for CPU inference.

### What to include

- Language and framework
- Build and test commands
- Key directory structure
- Coding conventions
- Available environment tools the agent should prefer

### What to avoid

- Full file listings or dependency lists
- Long descriptions of features
- Anything the agent can discover by reading files

## Environment Tools Awareness

Instead of modifying the system prompt (which costs tokens on every request), put environment tool awareness in your project context file. This costs nothing extra — the context budget is already allocated.

**Example `.gilgameshfile`:**

```
Go project. Module: github.com/myorg/myapp

Build: go build -o myapp .
Test: go test ./... -v -cover
Lint: golangci-lint run

This environment has rg (ripgrep), fd, bat, eza, fzf, zoxide installed.
Prefer rg over grep, fd over find. Use bat for syntax-highlighted previews.

Conventions: table-driven tests, Go stdlib only, no external deps.
Key paths: cmd/server/main.go, internal/api/, db/queries/
```

This tells the agent about available tools without adding to the base system prompt.

## Custom Tools

For frequently used environment commands, register them as custom tools in `.gilgamesh/tools.json`:

```json
[
  {
    "name": "search",
    "description": "Search file contents with ripgrep",
    "command": "rg --no-heading --line-number '{{pattern}}' {{path}}",
    "parameters": {
      "type": "object",
      "properties": {
        "pattern": { "type": "string", "description": "Regex pattern to search for" },
        "path": { "type": "string", "description": "Directory or file to search" }
      },
      "required": ["pattern"]
    }
  },
  {
    "name": "tree",
    "description": "Show directory tree structure",
    "command": "eza --tree --level={{depth}} {{path}}",
    "parameters": {
      "type": "object",
      "properties": {
        "path": { "type": "string", "description": "Root directory" },
        "depth": { "type": "string", "description": "Max depth level" }
      }
    }
  }
]
```

Custom tools use `{{param}}` template substitution and also set `GILGAMESH_<PARAM>` environment variables.

## Memory for Persistent Facts

Use `/remember` in the REPL to store environment facts that persist across sessions:

```
/remember This machine has 16 cores, 12 threads optimal for llama.cpp
/remember Prefer table-driven tests with testing stdlib
/remember Production DB is on port 5432, test DB on 5433
```

Memory entries are stored in `.gilgamesh/memory.json` and injected into the system prompt (~200 token cap). Use `/memory` to view, `/forget` to remove.

## Example Context Files

### Go Project

```
Go 1.25+ project. Zero external deps, stdlib only.
Build: go build -o myapp .
Test: go test ./... -v -cover -count=1
Key: cmd/main.go, internal/*, pkg/*
Style: table-driven tests, error wrapping, context propagation.
```

### Python Project

```
Python 3.12 project with uv for package management.
Install: uv sync
Test: uv run pytest -xvs
Lint: uv run ruff check .
Key: src/app/, tests/
Style: type hints, dataclasses over dicts, pytest fixtures.
```

### Rust Project

```
Rust project. Cargo workspace with 3 crates.
Build: cargo build --release
Test: cargo test --workspace
Key: crates/core/src/lib.rs, crates/cli/src/main.rs
Style: clippy clean, no unwrap in library code, thiserror for errors.
```

### Node.js Project

```
Node.js 22 with TypeScript. Package manager: bun.
Install: bun install
Test: bun test
Build: bun run build
Key: src/index.ts, src/routes/, src/db/
Style: strict TypeScript, zod for validation, vitest for tests.
```

Each example is under 500 tokens and gives the agent everything it needs to work effectively in the project.

## Token Budget

| Component | Tokens | Source |
|-----------|--------|--------|
| System prompt | ~300 | Fixed (agent/prompt.go) |
| Tool definitions | ~800 | 7 built-in + custom tools |
| Project context | ~500 | .gilgameshfile or .gilgamesh/context.md |
| Memory entries | ~200 | .gilgamesh/memory.json |
| **Total overhead** | **~1,800** | Before any user message |

On Qwen3.5-2B Q4_K_M at ~160 tok/s PP, this means the first response arrives in ~11 seconds. Keeping context and memory concise directly improves response latency.
