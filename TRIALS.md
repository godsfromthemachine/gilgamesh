# Model Trials & Benchmarks

Gilgamesh is designed for CPU inference with small local models. This document tracks model trialing work — benchmarking, evaluating, and tuning models for optimal agent performance.

## The Quest

Find the ultimate local coding setup: the best model + quantization + inference parameters for a responsive, reliable, tool-calling coding agent running entirely on CPU.

## Hardware

- **CPU**: AMD EPYC-Rome 16-core @ 2.0GHz
- **RAM**: 30GB
- **GPU**: None
- **Inference**: llama.cpp (CPU-only, AVX2, 16 threads)

## Models Under Test

| Model | Size | Quant | Disk | RAM | Status |
|-------|------|-------|------|-----|--------|
| Qwen3.5-0.8B | 0.8B | Q4_K_M | 508MB | ~1.5GB | Rejected — too unreliable |
| Qwen3.5-2B | 2B | Q4_K_M | 1.2GB | ~2.8GB | **Current default** |
| Qwen3.5-2B | 2B | Q8_0 | 1.9GB | ~3.5GB | Tested — marginal quality gain |
| Qwen3.5-4B | 4B | Q4_K_M | 2.6GB | ~4.5GB | Available |
| Qwen3.5-4B | 4B | Q8_0 | 4.2GB | ~5.5GB | **Current heavy** |
| Qwen3.5-9B | 9B | Q8_0 | 8.9GB | ~10GB | Slow but high quality |

## Benchmark Results

### Raw Inference Speed (llama-bench, pp512/tg128, 16 threads)

| Model | PP (tok/s) | TG (tok/s) |
|-------|-----------|-----------|
| Qwen3.5-2B Q4_K_M | **181** | **19** |
| Qwen3.5-2B Q8_0 | 130 | 15 |
| Qwen3.5-4B Q4_K_M | 95 | 11 |
| Qwen3.5-4B Q8_0 | 54 | 6.3 |
| Qwen3.5-9B Q8_0 | 28 | 3.2 |

### Agent Response Time (with ~1,600 token overhead)

| Model | First Response | Simple Task | Tool Task |
|-------|---------------|-------------|-----------|
| 2B Q4_K_M | ~7s | ~10s | ~15s |
| 4B Q8_0 | ~25s | ~35s | ~60s |

## Key Findings

### 2B Q4_K_M is the sweet spot
- 181 tok/s prompt processing — fast enough for interactive use
- 19 tok/s generation — readable streaming output
- Tool calling works reliably (glob, read, write, edit, bash)
- First response in ~7 seconds with gilgamesh's 1,600-token overhead
- 43% faster prompt processing than 8-thread config

### 0.8B is unusable for agent work
- Frequent tool call loops (repeated identical calls)
- Hallucinated tool names and arguments
- Could not reliably follow multi-step instructions
- Loop detection triggers constantly

### 4B Q8_0 is the quality ceiling
- Significantly better code generation quality
- Better at complex multi-step tasks and refactoring
- But 3x slower first response (~25s vs ~7s)
- Use for tasks where quality matters more than speed

### Q4_K_M vs Q8_0 tradeoff
- Q4_K_M: ~40% faster, marginally lower quality
- Q8_0: better at edge cases, longer coherent outputs
- For agent loops (tool calling), Q4_K_M is sufficient
- For complex code generation, Q8_0 is preferable

### Token budget is everything
- Competitors use 10,000-40,000 token system prompts — unusable on CPU
- Gilgamesh: ~1,600 tokens total overhead
- Every 1,000 extra tokens = ~5.5s added latency at 181 tok/s
- Context compaction at 12K tokens keeps interactions responsive

## Running Benchmarks

Gilgamesh includes a Go benchmark tool:

```bash
# Build gilgamesh first
go build -o gilgamesh .

# Benchmark default endpoint
go run ./cmd/bench

# Benchmark specific profile
go run ./cmd/bench -model heavy

# Benchmark all reachable endpoints
go run ./cmd/bench -all

# Custom endpoint
go run ./cmd/bench -endpoint http://127.0.0.1:8081/v1

# Verbose output
go run ./cmd/bench -v
```

### What the benchmark measures

1. **Health check** — endpoint latency
2. **Minimal prompt** — TTFT + generation speed with tiny prompt
3. **Tool call** — can the model emit valid tool calls?
4. **One-shot** — end-to-end gilgamesh `run` with simple question
5. **Edit task** — full agent loop: create file + edit it (write + edit tools)

## Inference Server Setup

Gilgamesh works with any OpenAI-compatible API. The reference setup uses llama.cpp:

```bash
# Build llama.cpp (CPU-only)
cmake -B build -DBUILD_SHARED_LIBS=OFF -DGGML_CUDA=OFF
cmake --build build --config Release -j$(nproc)

# Serve 2B model (fast/default)
./llama-server \
    --model Qwen3.5-2B-Q4_K_M.gguf \
    --port 8081 --host 127.0.0.1 \
    --ctx-size 65536 --threads 16 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 \
    --chat-template-kwargs '{"enable_thinking":false}'

# Serve 4B model (heavy)
./llama-server \
    --model Qwen3.5-4B-Q8_0.gguf \
    --port 8080 --host 127.0.0.1 \
    --ctx-size 65536 --threads 16 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 \
    --chat-template-kwargs '{"enable_thinking":false}'
```

### Key parameters

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| `--ctx-size` | 65536 | Full context window for Qwen3.5 |
| `--threads` | 16 | All cores for prompt processing |
| `--temp` | 0.6 | Low temperature for deterministic tool calls |
| `--top-p` | 0.95 | Nucleus sampling for coherent output |
| `--top-k` | 20 | Narrow vocabulary for focused generation |
| `--min-p` | 0.0 | No minimum probability threshold |
| `--chat-template-kwargs` | `{"enable_thinking":false}` | Disable reasoning tokens (saves token budget) |

## Model Configuration

Create `gilgamesh.json` in your project root:

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

## Future Trials

- [ ] Qwen3.5-4B Q4_K_M — faster 4B option, quality vs 2B Q4_K_M?
- [ ] IQ4_XS / IQ3_M quants — smaller memory footprint, quality impact?
- [ ] Context length impact — does shorter ctx-size improve speed?
- [ ] Thread count tuning — is 16 threads always optimal?
- [ ] Batch size tuning — effect on prompt processing speed
- [ ] New model families — Phi-4, Gemma 3, others that fit CPU constraints
- [ ] Speculative decoding — draft model (0.8B) + verify (4B)?
- [ ] Multi-model routing — simple tasks → 2B, complex → 4B automatically
