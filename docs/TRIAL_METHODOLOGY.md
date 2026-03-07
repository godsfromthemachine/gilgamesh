# Trial Methodology

Controlled protocol for benchmarking and evaluating local models with gilgamesh.

## Prerequisites

- llama.cpp built CPU-only (AVX2) — see [LOCAL_AI_SETUP.md](../LOCAL_AI_SETUP.md)
- Models in `local-ai/models/` (or set `MODEL_DIR`)
- `llama-bench` in `local-ai/bin/` (or set `LLAMA_BENCH`)
- gilgamesh binary built: `go build -o gilgamesh .`

## Controlled Trial Protocol

Every benchmark must follow this protocol for reproducible results:

1. **Stop all inference servers**: `pkill llama-server`
2. **Wait for clean CPU state**: no background inference, verify with `htop` or `btm`
3. **Start only the server under test** with documented parameters
4. **Run the benchmark**: `go run ./cmd/bench` (or `-all` for comparisons)
5. **Average multiple runs**: raw inference = 2 runs, agent benchmarks = 3 runs
6. **Same binary, same config, same test prompts** throughout

## Running Benchmarks

```bash
# Single profile (active from gilgamesh.json)
go run ./cmd/bench

# Specific profile
go run ./cmd/bench -model heavy

# All profiles with summary table
go run ./cmd/bench -all

# Include raw llama-bench pp/tg metrics
go run ./cmd/bench -raw

# JSON output for scripting
go run ./cmd/bench -json

# Save results to JSON log (appends)
go run ./cmd/bench -save bench-results.json

# Full trial run
go run ./cmd/bench -all -raw -save results.json

# Custom endpoint
go run ./cmd/bench -endpoint http://127.0.0.1:9090/v1

# Verbose output
go run ./cmd/bench -v
```

## What the Benchmark Measures

The bench suite measures six dimensions:

| Stage | What | How |
|-------|------|-----|
| 1. Health check | Endpoint latency | `GET /health` |
| 2. Raw inference | PP/TG tok/s | `llama-bench` binary (auto-detected) |
| 3. Minimal prompt | TTFT + generation speed | Tiny prompt via API |
| 4. Tool call | Valid tool call emission | Agent prompt requesting glob |
| 5. One-shot | End-to-end response | `gilgamesh run` with simple question |
| 6. Edit task | Full agent loop quality | Create file + edit it (write + edit tools) |

## Server Configuration for Trials

Standard server parameters for consistent benchmarking:

```bash
llama-server \
    --model <path-to-gguf> \
    --port <port> --host 127.0.0.1 \
    --ctx-size 16384 --threads 12 --batch-size 256 \
    -ctk q4_0 -ctv q4_0 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 \
    --chat-template-kwargs '{"enable_thinking":false}'
```

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| `--threads` | 12 | Optimal on 16-core EPYC (TG degrades at 16) |
| `--ctx-size` | 16384 | Covers agent needs, saves ~500MB vs 65K |
| `--batch-size` | 256 | Optimal PP; b=512 regresses (cache pressure) |
| `-ctk/-ctv` | q4_0 | KV cache quantization saves 5-7% RAM, no quality loss |
| `--temp` | 0.6 | Low temperature for deterministic tool calls |

## JSON Output Format

Results saved with `-save` for historical tracking:

```json
{
  "timestamp": "2026-03-06T19:45:00Z",
  "system": {"cpu": "AMD EPYC-Rome", "cores": 16, "ram": "30Gi"},
  "profile": "default",
  "endpoint": "http://127.0.0.1:8081/v1",
  "health": {"latency_ms": 0, "status": 200},
  "raw": {"pp_tok_s": 172, "tg_tok_s": 19.2, "threads": 16},
  "minimal": {"elapsed_ms": 658, "prompt_tokens": 33, "completion_tokens": 8},
  "tool_call": {"elapsed_ms": 3151, "tool_name": "glob", "tool_calls": 1},
  "one_shot": {"elapsed_ms": 3330},
  "edit": {"elapsed_ms": 88359, "tool_calls": 6, "pass": true}
}
```

## How to Add a New Model

1. Download the GGUF file to `local-ai/models/<ModelName>/`
2. Add a profile to `gilgamesh.json`:
   ```json
   "trial": {
     "name": "new-model",
     "endpoint": "http://127.0.0.1:8082/v1",
     "api_key": "sk-local"
   }
   ```
3. Start the server with standard parameters (see above)
4. Run: `go run ./cmd/bench -model trial -raw -save results.json`
5. Compare: `go run ./cmd/bench -all -raw`
6. Document findings in `TRIALS.md`

## Interpreting Results

Key metrics to compare:

| Metric | Good | Acceptable | Poor |
|--------|------|-----------|------|
| PP tok/s | >150 | 60-150 | <60 |
| TG tok/s | >15 | 7-15 | <7 |
| Tool call | <5s | 5-10s | >10s |
| Edit task | PASS, <60s | PASS, <120s | FAIL or >120s |
| Edit tool calls | 2 | 3-5 | >5 (inefficient) |

## Monitoring During Trials

Use `btm` (bottom) for real-time monitoring of CPU, memory, and process metrics during benchmark runs. Key things to watch:

- CPU utilization should be ~75% (12/16 cores) during inference
- RSS memory should match expected model + KV cache size
- No unexpected background processes competing for CPU
