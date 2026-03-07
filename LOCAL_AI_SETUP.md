# Local AI Setup Guide

Complete guide for setting up local AI inference with gilgamesh using llama.cpp and Qwen3.5 models.

## Prerequisites

**Hardware:**
- CPU with AVX2 support (most modern x86 CPUs)
- 4GB+ RAM for single model, 8GB+ for dual-model serving
- No GPU required — gilgamesh is designed for CPU inference

**Software:**
- cmake 3.14+, C/C++ compiler (gcc/clang)
- git, curl

## 1. Build llama.cpp

```bash
git clone https://github.com/ggml-org/llama.cpp
cd llama.cpp

# CPU-only build (no CUDA, no shared libs)
cmake -B build -DBUILD_SHARED_LIBS=OFF -DGGML_CUDA=OFF
cmake --build build --config Release -j$(nproc)

# Binaries are in build/bin/
ls build/bin/llama-server build/bin/llama-bench build/bin/llama-cli
```

Copy or symlink the binaries somewhere convenient:

```bash
mkdir -p ~/local-ai/bin
cp build/bin/llama-{server,bench,cli} ~/local-ai/bin/
```

## 2. Download Models

Download Qwen3.5 GGUF models from HuggingFace. Recommended setup:

| Model | Quant | Size | Role | Why |
|-------|-------|------|------|-----|
| **Qwen3.5-2B** | Q4_K_M | ~1.5GB | Default/fast | Sweet spot — 160 tok/s PP, 19 tok/s TG, reliable tool calls |
| **Qwen3.5-4B** | Q4_K_M | ~2.8GB | Heavy | Quality ceiling — fewer tool calls needed, better planning |

```bash
mkdir -p ~/local-ai/models/Qwen3.5-{2B,4B}

# Download 2B Q4_K_M (default profile)
curl -L -o ~/local-ai/models/Qwen3.5-2B/qwen3.5-2b-q4_k_m.gguf \
  "https://huggingface.co/unsloth/Qwen3.5-2B-GGUF/resolve/main/Qwen3.5-2B-Q4_K_M.gguf"

# Download 4B Q4_K_M (heavy profile)
curl -L -o ~/local-ai/models/Qwen3.5-4B/qwen3.5-4b-q4_k_m.gguf \
  "https://huggingface.co/unsloth/Qwen3.5-4B-GGUF/resolve/main/Qwen3.5-4B-Q4_K_M.gguf"
```

### Models we tested and rejected

| Model | Verdict | Reason |
|-------|---------|--------|
| Qwen3.5-0.8B | Rejected | Unreliable tool calls, loop detection triggers constantly |
| Qwen3.5-9B Q8_0 | Not worth it | Same 2-tool efficiency as 4B but 40-70% slower |
| 4B Q8_0 vs Q4_K_M | Q4_K_M wins | Same inference speed (memory bandwidth bottleneck), saves 1.6GB disk |

See [TRIALS.md](TRIALS.md) for detailed benchmark data.

## 3. Start Inference Servers

### Single model (simple setup)

```bash
~/local-ai/bin/llama-server \
    --model ~/local-ai/models/Qwen3.5-2B/qwen3.5-2b-q4_k_m.gguf \
    --port 8081 --host 127.0.0.1 \
    --ctx-size 16384 --threads 12 --batch-size 256 \
    -ctk q4_0 -ctv q4_0 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 \
    --chat-template-kwargs '{"enable_thinking":false}'
```

### Dual model (recommended)

Run both servers for model switching via `/model` command:

```bash
# Terminal 1: 2B on :8081 (default/fast)
~/local-ai/bin/llama-server \
    --model ~/local-ai/models/Qwen3.5-2B/qwen3.5-2b-q4_k_m.gguf \
    --port 8081 --host 127.0.0.1 \
    --ctx-size 16384 --threads 12 --batch-size 256 \
    -ctk q4_0 -ctv q4_0 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 \
    --chat-template-kwargs '{"enable_thinking":false}'

# Terminal 2: 4B on :8080 (heavy)
~/local-ai/bin/llama-server \
    --model ~/local-ai/models/Qwen3.5-4B/qwen3.5-4b-q4_k_m.gguf \
    --port 8080 --host 127.0.0.1 \
    --ctx-size 16384 --threads 12 --batch-size 256 \
    -ctk q4_0 -ctv q4_0 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 \
    --chat-template-kwargs '{"enable_thinking":false}'
```

**RAM budget (dual-model):** ~2GB (2B) + ~4.5GB (4B) = ~6.5GB total.

When running both servers, consider using 8 threads each instead of 12 to avoid contention.

### Parameter rationale

| Parameter | Value | Why |
|-----------|-------|-----|
| `--threads 12` | 12 (not 16) | On 16-core EPYC: TG degrades 30% at 16 threads due to contention |
| `--ctx-size 16384` | 16K | Saves ~500MB vs 65K default. Agent compacts at 12K — 16K is sufficient |
| `--batch-size 256` | 256 | Optimal for PP. b=512 regresses due to L3 cache pressure |
| `-ctk q4_0 -ctv q4_0` | q4_0 KV cache | Saves 5-7% RAM with no quality loss on Qwen3.5 |
| `--temp 0.6` | Low temp | Deterministic tool calls, consistent output |
| `enable_thinking: false` | No reasoning | Saves token budget — thinking tokens add overhead without benefit for tool calls |

## 4. Configure Gilgamesh

Create `gilgamesh.json` in your project root (or `~/.config/gilgamesh/gilgamesh.json` for global config):

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

Switch models mid-session with `/model heavy` or start with `./gilgamesh -m heavy`.

## 5. Verify Setup

```bash
# Check server health
curl -s http://127.0.0.1:8081/health | head -1

# Build and run gilgamesh
go build -o gilgamesh .
./gilgamesh

# Or run benchmarks to validate performance
go run ./cmd/bench           # bench active profile
go run ./cmd/bench -all      # bench all profiles with summary
```

### Expected performance (AMD EPYC 16-core, 12 threads)

| Model | PP (tok/s) | TG (tok/s) | First Response |
|-------|-----------|-----------|----------------|
| 2B Q4_K_M | ~160 | ~19 | ~10s |
| 4B Q4_K_M | ~66 | ~10 | ~25s |

The 4B model compensates for slower inference with better planning — it often uses fewer tool calls (2 vs 5-9 for the same edit task), making it competitive in wall-clock time.

## Using gilgamesh's local-ai directory

For convenience, gilgamesh supports a `local-ai/` directory (gitignored) at its project root:

```
gilgamesh/
└── local-ai/
    ├── bin/           # llama-server, llama-bench, llama-cli
    └── models/        # GGUF model files
```

The benchmark tool (`cmd/bench/`) auto-detects these paths. Set `LLAMA_BENCH` and `MODEL_DIR` environment variables to override.

## Further Reading

- [TRIALS.md](TRIALS.md) — Detailed model trial results, benchmark data, and findings
- [README.md](README.md) — Gilgamesh features and usage
- [gilgamesh.example.json](gilgamesh.example.json) — Example configuration file
