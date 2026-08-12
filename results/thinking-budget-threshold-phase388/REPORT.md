# Phase 388 — Thinking Budget Threshold Campaign

**Date:** 2026-08-12 21:48 +0000
**Total trials:** 42

**Hypothesis:** qwen3.6-27b needs at least 384-512 max_tokens to emit
both thinking tokens and the structured DATE:/SOURCE: answer.
Below that, finish_reason=length truncates before the answer.

**Models:** qwen/qwen3.6-27b (thinking), llama-3.3-70b-versatile (control)

| Model | Max Tok | N | Fmt OK | Sem OK | Avg Lat ms | Avg Comp Tok | Avg Think Tok | Avg Ans Tok |
|-------|---------|---|--------|--------|------------|--------------|---------------|-------------|
| llama-3.3-70b-versatile | 128 | 3 | 3 | 3 | 403 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 256 | 3 | 3 | 3 | 408 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 384 | 3 | 3 | 3 | 274 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 512 | 3 | 3 | 3 | 277 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 768 | 3 | 3 | 3 | 297 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 1024 | 3 | 3 | 3 | 275 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 2048 | 3 | 3 | 3 | 405 | 16 | 0 | 0 |
| qwen/qwen3.6-27b | 128 | 3 | 3 | 0 | 473 | 128 | 100 | 19 |
| qwen/qwen3.6-27b | 256 | 3 | 3 | 0 | 725 | 256 | 100 | 96 |
| qwen/qwen3.6-27b | 384 | 3 | 3 | 0 | 1139 | 384 | 100 | 174 |
| qwen/qwen3.6-27b | 512 | 3 | 3 | 3 | 1293 | 512 | 100 | 300 |
| qwen/qwen3.6-27b | 768 | 3 | 3 | 3 | 1837 | 768 | 100 | 506 |
| qwen/qwen3.6-27b | 1024 | 3 | 2 | 0 | 1696 | 683 | 67 | 466 |
| qwen/qwen3.6-27b | 2048 | 3 | 0 | 0 | 37 | 0 | 0 | 0 |
