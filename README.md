<p align="center">
  <img src="assets/logo.svg" alt="gilgamesh" width="360"/>
</p>

<p align="center">
  A local AI-powered coding agent that takes a test-driven approach to software engineering.<br/>
  Built for CPU inference with lean token overhead.<br/>
  Part of the <a href="https://github.com/godsfromthemachine">Gods from the Machine</a> project.
</p>

---

## What is this?

Gilgamesh is an interactive CLI agent that connects to a local llama.cpp server (or any OpenAI-compatible endpoint) and provides tool-calling capabilities for software engineering tasks. It's designed to run on CPU with small models (Qwen3.5 2B/4B) by keeping total prompt overhead under ~1,500 tokens.

Features:
- **7 built-in tools**: read, write, edit, bash, grep, glob, test
- **Streaming SSE**: tokens stream to terminal as they arrive
- **Multi-model profiles**: switch between fast/default/heavy models mid-session
- **Skills system**: reusable prompt templates (`.gilgamesh/skills/*.md`)
- **Hook system**: pre/post tool execution hooks (`.gilgamesh/hooks.json`)
- **Session logging**: JSONL session logs with distill summaries
- **Loop detection**: detects and breaks out of repeated tool calls
- **Context compaction**: automatically trims old tool results to stay within context limits

## Quick start

```bash
# Build
go build -o gilgamesh .

# Interactive mode (connects to default model endpoint)
./gilgamesh

# One-shot mode
./gilgamesh run "list all Go files in this directory"

# Use a specific model profile
./gilgamesh -m heavy run "refactor this function"
```

## Configuration

Create `gilgamesh.json` in your project root or `~/.config/gilgamesh/gilgamesh.json`:

```json
{
  "models": {
    "fast": {
      "name": "qwen3.5-2b",
      "endpoint": "http://127.0.0.1:8081/v1",
      "api_key": "sk-local"
    },
    "default": {
      "name": "qwen3.5-2b",
      "endpoint": "http://127.0.0.1:8081/v1",
      "api_key": "sk-local"
    },
    "heavy": {
      "name": "qwen3.5-4b",
      "endpoint": "http://127.0.0.1:8080/v1",
      "api_key": "sk-local"
    }
  },
  "active_model": "default"
}
```

## Project context

Add a `.gilgameshfile` or `.gilgamesh/context.md` to your project root to inject project-specific context into the system prompt.

## Skills

Drop `.md` files into `.gilgamesh/skills/` (project-local) or `~/.config/gilgamesh/skills/` (global). Use `{{args}}` for argument substitution.

```markdown
# Build and test
Build the project and run all tests.
```

Invoke with `/skillname` or `/skillname args here`.

## Interactive commands

| Command | Description |
|---------|-------------|
| `/model [fast\|default\|heavy]` | Switch model |
| `/clear` | Reset conversation context |
| `/skills` | List available skills |
| `/tokens` | Show estimated context size |
| `/session` | Show session log path |
| `/distill [path]` | Summarize a session |
| `/exit` | Quit |

## MCP Server

Gilgamesh runs as an [MCP](https://modelcontextprotocol.io/) server, exposing its tools to any MCP-compatible client (Claude Desktop, VS Code, other agents) over stdio.

```bash
gilgamesh mcp
```

Configure in Claude Desktop's `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "gilgamesh": {
      "command": "/path/to/gilgamesh",
      "args": ["mcp"]
    }
  }
}
```

Implements `initialize`, `tools/list`, and `tools/call` via JSON-RPC 2.0 over stdio.

## HTTP API

Run gilgamesh as an HTTP server for programmatic access:

```bash
gilgamesh serve              # default port :7777
gilgamesh serve -p 8888     # custom port
```

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/tools` | List all tools with schemas |
| POST | `/api/tools/{name}` | Execute a tool (JSON body = args) |
| POST | `/api/chat` | Agent conversation (SSE streaming) |

```bash
# List tools
curl http://localhost:7777/api/tools

# Execute a tool directly
curl -X POST http://localhost:7777/api/tools/read -d '{"path": "main.go"}'

# Chat with the agent (SSE stream)
curl -N -X POST http://localhost:7777/api/chat -d '{"message": "list all Go files"}'
```

## Architecture

```
gilgamesh/
├── main.go           # CLI entry, REPL, subcommand dispatch
├── agent/
│   ├── agent.go      # Core agent loop + event-based variant
│   └── prompt.go     # System prompt (~300 tokens)
├── llm/
│   └── client.go     # OpenAI-compatible streaming SSE client
├── tools/
│   ├── registry.go   # Tool registration, dispatch, enumeration
│   ├── read.go       # Read file contents
│   ├── write.go      # Write/create files
│   ├── edit.go       # Find-and-replace editing
│   ├── bash.go       # Shell command execution
│   ├── grep.go       # Content search
│   ├── glob.go       # File pattern matching
│   └── test.go       # Go test runner (packages, filters, coverage)
├── mcp/
│   ├── protocol.go   # JSON-RPC 2.0 + MCP protocol types
│   └── server.go     # MCP stdio server
├── server/
│   └── server.go     # HTTP API server
├── config/           # JSON config loader
├── context/          # Project context + skills
├── hooks/            # Pre/post tool execution hooks
└── session/          # JSONL session logging
```

## License

MIT
