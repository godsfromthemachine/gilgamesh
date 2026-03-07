# Profile system during task

Monitor CPU and memory usage while running: {{args}}

Use `btm` (bottom) or `top -bn1` for system metrics. Track:
- Overall CPU utilization (should be ~75% with 12/16 cores active)
- llama-server RSS memory (compare against expected model + KV cache size)
- Per-process CPU% for llama-server
- Any unexpected background processes competing for resources

Report: total elapsed time, average/peak CPU%, per-process RSS memory.
