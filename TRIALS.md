# Model Trials & Benchmarks

Gilgamesh is designed for CPU inference with small local models. This document tracks model trialing work — benchmarking, evaluating, and tuning models for optimal agent performance.

## The Quest

Find the ultimate local coding setup: the best model + quantization + inference parameters for a responsive, reliable, tool-calling coding agent running entirely on CPU.

## Hardware

- **CPU**: AMD EPYC-Rome 16-core @ 2.0GHz
- **RAM**: 30GB
- **GPU**: None
- **Inference**: llama.cpp (CPU-only, AVX2, 12 threads optimal)

## Models Under Test

| Model | Size | Quant | Disk | RAM | Status |
|-------|------|-------|------|-----|--------|
| Qwen3.5-0.8B | 0.8B | Q4_K_M | 497MB | ~1.5GB | Rejected — too unreliable for agent work |
| Qwen3.5-2B | 2B | Q4_K_M | 1.18GB | ~2.8GB | **Current default** — sweet spot |
| Qwen3.5-2B | 2B | Q8_0 | 1.86GB | ~3.5GB | Tested — marginal quality gain, 27% slower |
| Qwen3.5-4B | 4B | Q4_K_M | 2.54GB | ~4.5GB | Tested — same speed as Q8_0, saves disk |
| Qwen3.5-4B | 4B | Q8_0 | 4.17GB | ~5.5GB | **Current heavy** — quality ceiling |
| Qwen3.5-9B | 9B | Q8_0 | 8.9GB | ~10GB | **Not worth it** — same efficiency as 4B, 40-70% slower |

## Benchmark Results

### Raw Inference Speed (llama-bench, pp512/tg32, 16 threads)

Measured 2026-03-06 with gilgamesh bench suite (`go run ./cmd/bench -raw`).

| Model | Disk | PP (tok/s) | TG (tok/s) | PP Relative | TG Relative |
|-------|------|-----------|-----------|-------------|-------------|
| Qwen3.5-0.8B Q4_K_M | 497MB | **268** | **22.7** | 1.56x | 1.27x |
| Qwen3.5-2B Q4_K_M | 1.18GB | **172** | **19.2** | 1.00x (baseline) | 1.00x |
| Qwen3.5-2B Q8_0 | 1.86GB | 132 | 14.1 | 0.77x | 0.74x |
| Qwen3.5-4B Q4_K_M | 2.54GB | 72 | 7.4 | 0.42x | 0.39x |
| Qwen3.5-4B Q8_0 | 4.17GB | 72 | 8.0 | 0.42x | 0.42x |

### Agent Benchmarks (gilgamesh bench suite)

Full agent benchmarks with gilgamesh's ~1,600 token overhead. Measured via `go run ./cmd/bench -all`.

| Model | Minimal Prompt | Tool Call | One-Shot | Edit Task |
|-------|---------------|-----------|----------|-----------|
| 2B Q4_K_M | 650-840ms | 3.1-3.4s | 1.1-8.1s | 34-146s (PASS, occasionally FAIL) |
| 4B Q8_0 | 2.7s | 8.1s | 23s | 156s (PASS, reliable) |

### 4B Q4_K_M vs Q8_0 Comparison

| Metric | 4B Q4_K_M | 4B Q8_0 | Difference |
|--------|-----------|---------|------------|
| PP tok/s | 73 | 72 | **Same** |
| TG tok/s | 7.3 | 8.0 | ~10% slower gen |
| Disk | 2.54GB | 4.17GB | **39% smaller** |
| Minimal prompt | 1.7-1.8s | 2.7s | Q4_K_M faster (less data to load) |
| Tool call | 7.5s | 8.1s | Similar |
| Edit task | 60-96s, 2 tools, PASS | 156s, 4 tools, PASS | Q4_K_M more efficient |

**Verdict**: 4B Q4_K_M is the better 4B option. Same inference speed, 39% smaller on disk, and actually performs better in agent benchmarks (fewer tool calls, faster completion). Q8_0 offers no meaningful advantage at this model size on CPU. **Recommendation: use 4B Q4_K_M as the heavy profile instead of Q8_0.**

### 2B vs 4B Agent Efficiency Comparison

| Metric | 2B Q4_K_M | 4B Q4_K_M | Tradeoff |
|--------|-----------|-----------|----------|
| Minimal prompt | 650-840ms | 1.7-1.8s | 2B is 2.5x faster |
| Tool call | 3.1-3.4s | 7.5s | 2B is 2.3x faster |
| One-shot | 1-8s | ~20s | 2B is 3-20x faster (variable) |
| Edit task time | 34-146s | 60-96s | 4B is more consistent |
| Edit tool calls | 3-9 calls | 2 calls | 4B is more efficient |
| Edit reliability | PASS (occasionally FAIL) | PASS (consistent) | 4B is more reliable |

**Key insight**: The 4B model compensates for slower inference with better planning — it uses fewer tool calls to complete the same task. The net result: 4B edit tasks can actually be faster than 2B when 2B makes many attempts.

## Key Findings

### 2B Q4_K_M is the sweet spot
- 172 tok/s prompt processing — fast enough for interactive use
- 19.2 tok/s generation — readable streaming output
- Tool calling works reliably (glob, read, write, edit, bash)
- First response in ~3-8 seconds with gilgamesh's 1,600-token overhead
- Edit task passes but occasionally fails (SLM reliability)

### 0.8B is unusable for agent work
- Fastest raw inference (268 pp, 22.7 tg) but the speed is wasted
- Frequent tool call loops (repeated identical calls)
- Hallucinated tool names and arguments
- Loop detection triggers constantly
- Could not reliably follow multi-step instructions

### 4B Q8_0 is the quality ceiling
- Significantly better code generation quality
- More reliable at complex multi-step tasks and refactoring
- But 2.4x slower first response vs 2B
- Edit task passes consistently — higher reliability
- Use for tasks where quality matters more than speed

### Q4_K_M vs Q8_0 tradeoff
- **2B**: Q4_K_M is 30% faster than Q8_0 — clear win for speed
- **4B**: Q4_K_M and Q8_0 are nearly identical speed — use Q4_K_M to save disk
- For agent loops (tool calling), Q4_K_M is sufficient at both sizes
- For complex code generation, Q8_0 may produce marginally better output

### Token budget is everything
- Competitors use 10,000-40,000 token system prompts — unusable on CPU
- Gilgamesh: ~1,600 tokens total overhead (system ~300, tools ~800, context ~500)
- Every 1,000 extra tokens = ~5.8s added latency at 172 tok/s
- Context compaction at 12K tokens keeps interactions responsive

## Running Benchmarks

Gilgamesh includes a pure Go benchmark suite:

```bash
# Build gilgamesh first
go build -o gilgamesh .

# Benchmark active profile from config
go run ./cmd/bench

# Benchmark specific profile
go run ./cmd/bench -model heavy

# Benchmark all configured profiles with summary table
go run ./cmd/bench -all

# Include raw llama-bench metrics (requires llama-bench binary)
go run ./cmd/bench -raw

# JSON output for scripting/tracking
go run ./cmd/bench -json

# Save results to a JSON log file (appends)
go run ./cmd/bench -save bench-results.json

# Combine flags
go run ./cmd/bench -all -raw -save results.json

# Custom endpoint
go run ./cmd/bench -endpoint http://127.0.0.1:9090/v1

# Verbose output (show errors, raw llama-bench lines)
go run ./cmd/bench -v
```

### What the benchmark measures

1. **Health check** — endpoint latency
2. **Raw inference** — llama-bench pp/tg tok/s (requires `llama-bench` binary in `local-ai/bin/` or `LLAMA_BENCH` env var)
3. **Minimal prompt** — TTFT + generation speed with tiny prompt via API
4. **Tool call** — can the model emit valid tool calls? Measures tool name + call count
5. **One-shot** — end-to-end gilgamesh `run` with simple question
6. **Edit task** — full agent loop: create file + edit it (write + edit tools)

### JSON output format

Results can be saved to a JSON log file for historical tracking:

```json
{
  "timestamp": "2026-03-06T19:45:00Z",
  "system": {"cpu": "AMD EPYC-Rome", "cores": 16, "ram": "30Gi"},
  "profile": "default",
  "endpoint": "http://127.0.0.1:8081/v1",
  "health": {"latency_ms": 0, "status": 200},
  "raw": {"pp_tok_s": 172, "tg_tok_s": 19.2, "threads": 16},
  "minimal": {"elapsed_ms": 658, "prompt_tokens": 33, "completion_tokens": 8, "tok_per_sec": 12.2},
  "tool_call": {"elapsed_ms": 3151, "tool_name": "glob", "tool_calls": 1},
  "one_shot": {"elapsed_ms": 3330},
  "edit": {"elapsed_ms": 88359, "tool_calls": 6, "pass": true}
}
```

## Inference Server Setup

Gilgamesh works with any OpenAI-compatible API. The reference setup uses llama.cpp:

```bash
# Build llama.cpp (CPU-only)
cmake -B build -DBUILD_SHARED_LIBS=OFF -DGGML_CUDA=OFF
cmake --build build --config Release -j$(nproc)

# Serve 2B model (fast/default) — 12 threads, b=256 optimal
./llama-server \
    --model Qwen3.5-2B-Q4_K_M.gguf \
    --port 8081 --host 127.0.0.1 \
    --ctx-size 16384 --threads 12 --batch-size 256 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 \
    --chat-template-kwargs '{"enable_thinking":false}'

# Serve 4B model (heavy) — Q4_K_M recommended over Q8_0 (same speed, smaller)
./llama-server \
    --model Qwen3.5-4B-Q4_K_M.gguf \
    --port 8080 --host 127.0.0.1 \
    --ctx-size 16384 --threads 12 --batch-size 256 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 \
    --chat-template-kwargs '{"enable_thinking":false}'
```

### Key parameters

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| `--ctx-size` | 16384 | Covers agent needs (12K compaction threshold), saves ~500MB vs 65K |
| `--threads` | 12 | Optimal on 16-core EPYC — 30% better TG than 16 threads |
| `--batch-size` | 256 | Optimal for PP; b=512 regresses due to cache pressure |
| `--temp` | 0.6 | Low temperature for deterministic tool calls |
| `--top-p` | 0.95 | Nucleus sampling for coherent output |
| `--top-k` | 20 | Narrow vocabulary for focused generation |
| `--min-p` | 0.0 | No minimum probability threshold |
| `--chat-template-kwargs` | `{"enable_thinking":false}` | Disable reasoning tokens (saves token budget) |

### Local AI directory structure

For convenience, gilgamesh uses a `local-ai/` directory (gitignored) for model files and binaries:

```
local-ai/
├── bin/
│   ├── llama-bench     # Benchmark binary
│   ├── llama-server    # Inference server
│   └── llama-cli       # CLI interface
└── models/
    ├── Qwen3.5-0.8B/   # Q4_K_M, Q8_0
    ├── Qwen3.5-2B/     # Q4_K_M (default), Q8_0
    ├── Qwen3.5-4B/     # Q4_K_M, Q8_0 (heavy)
    └── Qwen3.5-9B/     # Q8_0
```

The bench tool auto-detects these paths. Set `LLAMA_BENCH` and `MODEL_DIR` env vars to override.

## Model Configuration

Create `gilgamesh.json` in your project root (see `gilgamesh.example.json`):

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

Switch models mid-session with `/model heavy` or at launch with `-m fast`.

### Thread Count Tuning (New Finding)

Tested with all servers stopped, clean CPU, 2 runs averaged. Results are consistent across model sizes.

**Qwen3.5-2B Q4_K_M:**

| Threads | PP (tok/s) | TG (tok/s) | PP vs 16 | TG vs 16 |
|---------|-----------|-----------|----------|----------|
| 8 | 121 | 18.9 | 77% | **117%** |
| 12 | 156 | **21.0** | 100% | **130%** |
| 16 | 156 | 16.1 | baseline | baseline |

**Qwen3.5-4B Q4_K_M:**

| Threads | PP (tok/s) | TG (tok/s) | PP vs 16 | TG vs 16 |
|---------|-----------|-----------|----------|----------|
| 8 | 46 | 8.2 | 71% | **110%** |
| 12 | 63 | **9.1** | 97% | **122%** |
| 16 | 65 | 7.5 | baseline | baseline |

**Key findings:**
- **PP saturates at 12 threads** — going to 16 gives only 0-3% more PP
- **TG degrades at 16 threads** — 23% worse on 2B, 22% worse on 4B
- **12 threads is the sweet spot** for both PP and TG on this 16-core EPYC
- Root cause: at 16 threads, thread contention and NUMA overhead outweigh parallelism for the sequential token generation workload
- **Recommendation**: Use `--threads 12` instead of `--threads 16` for all server profiles

### Batch Size Tuning (New Finding)

Tested with llama-bench at 12 threads, pp512/tg32, 2 runs averaged. Batch size (`-b`) controls how many tokens are processed simultaneously during prompt evaluation.

**Qwen3.5-2B Q4_K_M:**

| Batch Size | PP (tok/s) | TG (tok/s) | PP vs b=512 |
|-----------|-----------|-----------|-------------|
| 32 | 120.8 | 20.3 | 77% |
| 64 | 146.6 | 21.2 | 93% |
| 128 | 153.1 | 21.7 | 97% |
| **256** | **160.1** | **22.1** | **102%** |
| 512 | 157.4 | 20.2 | baseline |

**Qwen3.5-4B Q4_K_M:**

| Batch Size | PP (tok/s) | TG (tok/s) | PP vs b=512 |
|-----------|-----------|-----------|-------------|
| 32 | 52.3 | 9.4 | 81% |
| 64 | 62.5 | 9.4 | 97% |
| 128 | 63.2 | 9.5 | 98% |
| **256** | **65.6** | **9.6** | **102%** |
| 512 | 64.5 | 9.0 | baseline |

**Key findings:**
- **b=256 is optimal** for both model sizes — 2-3% faster PP than b=512
- **b=512 actually regresses** on both PP and TG (cache pressure)
- b=32 is 20-25% slower — too small for efficient SIMD utilization
- **TG is also affected** — b=256 gives 9% better TG than b=512 on 2B
- **Recommendation**: Use `--batch-size 256` for llama-server

### 9B Q8_0 Agent Benchmarks (New Finding)

Tested with llama-server at 12 threads, ctx-size 16384, b=256. The question: is 9B quality worth the slowdown?

**Raw inference:**

| Model | PP (tok/s) | TG (tok/s) | PP vs 2B | TG vs 2B |
|-------|-----------|-----------|----------|----------|
| 2B Q4_K_M | 160.1 | 22.1 | 1.00x | 1.00x |
| 4B Q4_K_M | 65.6 | 9.6 | 0.41x | 0.43x |
| 9B Q8_0 | 30.3 | 5.7 | 0.19x | 0.26x |

**Agent benchmarks:**

| Metric | 2B Q4_K_M | 4B Q4_K_M | 9B Q8_0 |
|--------|-----------|-----------|---------|
| Minimal prompt | 650-840ms | 1.7-1.8s | 2.2s |
| Tool call | 3.1-3.4s | 7.5s | 12.5s |
| One-shot | 1-8s | ~20s | 42.3s |
| Edit task time | 34-146s | 60-96s | 132s |
| Edit tool calls | 3-9 | 2 | 2 |
| Edit reliability | PASS (flaky) | PASS | PASS |

**Verdict**: 9B is **not worth it** on this CPU. It produces the same 2-tool-call efficiency as 4B but takes 40-70% longer. The one-shot response at 42s and TG of 5.7 tok/s makes interactive use painful. The 4B Q4_K_M remains the best quality/speed tradeoff for heavy tasks.

### Dual-Model Serving Optimization

With 12 threads optimal for single-model serving, this opens the possibility of running **two models simultaneously** on 16 cores (e.g., 8 threads each) for multi-model routing:

| Config | 2B TG | 4B TG | Use Case |
|--------|-------|-------|----------|
| Single 12t | 21.0 | 9.1 | Best single-model performance |
| Dual 8t+8t | 18.9 + 8.2 | — | Run both 2B + 4B simultaneously |

Running both models at 8 threads each still gives usable TG (18.9 for 2B, 8.2 for 4B) — enabling future multi-model routing where simple tasks go to the faster 2B and complex tasks go to the 4B.

### Context Length Impact (New Finding)

Gilgamesh compacts context at ~12K tokens, so 65536 ctx is rarely needed. Testing memory impact of reduced ctx-size on 2B Q4_K_M:

| ctx-size | RAM (MB) | vs 65536 | Practical Capacity |
|----------|----------|----------|--------------------|
| 8192 | 1,990 | **-672MB (25% less)** | ~8K tokens (tight) |
| 65536 | 2,662 | baseline | ~65K tokens (overkill) |

**Findings:**
- KV cache for full 65K context adds ~672MB RAM
- Gilgamesh never needs >12K tokens (compaction threshold)
- `--ctx-size 16384` would save ~500MB while covering all practical agent usage
- **Recommendation**: Use `--ctx-size 16384` for memory-constrained setups, `65536` only if running long multi-turn sessions without compaction

## Future Trials

- [x] ~~4B Q4_K_M agent benchmarks~~ — DONE: better than Q8_0, recommend as heavy profile
- [x] ~~Thread count tuning~~ — DONE: 12 threads optimal, 16 threads hurts TG
- [x] ~~Context length impact~~ — DONE: 65K ctx adds 672MB RAM, 16K is sufficient
- [ ] IQ4_XS / IQ3_M quants — smaller memory footprint, quality impact?
- [x] ~~Context length impact~~ — DONE: 65K ctx adds 672MB RAM, 16K sufficient for agent
- [x] ~~Thread count tuning~~ — DONE: 12 threads optimal on 16-core EPYC
- [x] ~~Batch size tuning~~ — DONE: b=256 optimal, b=512 regresses (cache pressure)
- [x] ~~9B Q8_0 agent benchmarks~~ — DONE: not worth it, same efficiency as 4B but 40-70% slower
- [ ] New model families — Phi-4, Gemma 3, others that fit CPU constraints
- [ ] Speculative decoding — draft model (0.8B) + verify (4B)?
- [ ] Multi-model routing — simple tasks → 2B, complex → 4B automatically
- [ ] Flash attention impact on CPU — if llama.cpp supports it
- [ ] KV cache quantization — reduce memory for longer context
