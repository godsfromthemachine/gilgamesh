# Start inference server

Start llama-server for the {{args}} model profile.

Use these optimal parameters:
- `--ctx-size 16384 --threads 12 --batch-size 256 -ctk q4_0 -ctv q4_0`
- `--temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0`
- `--chat-template-kwargs '{"enable_thinking":false}'`

Port assignments: 2B (fast/default) on :8081, 4B (heavy) on :8080.

Steps:
1. Check if llama-server is in local-ai/bin/ or on PATH
2. Find the GGUF model file in local-ai/models/
3. Start the server in background with appropriate port
4. Health check: `curl http://127.0.0.1:<port>/health`
5. Report success or failure
