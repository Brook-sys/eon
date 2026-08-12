# Phase 388 — Thinking Budget Threshold Campaign

**Date:** 2026-08-12 19:47 +0000
**Total trials:** 42

**Hypothesis:** qwen3.6-27b needs at least 384-512 max_tokens to emit
both thinking tokens and the structured DATE:/SOURCE: answer.
Below that, finish_reason=length truncates before the answer.

**Models:** qwen/qwen3.6-27b (thinking), llama-3.3-70b-versatile (control)

| Model | Max Tok | N | Fmt OK | Sem OK | Avg Lat ms | Avg Comp Tok | Avg Think Tok | Avg Ans Tok |
|-------|---------|---|--------|--------|------------|--------------|---------------|-------------|
| llama-3.3-70b-versatile | 128 | 3 | 3 | 3 | 275 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 256 | 3 | 3 | 3 | 286 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 384 | 3 | 3 | 3 | 278 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 512 | 3 | 3 | 3 | 280 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 768 | 3 | 3 | 3 | 286 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 1024 | 3 | 3 | 3 | 316 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 2048 | 3 | 3 | 3 | 303 | 16 | 0 | 0 |
| qwen/qwen3.6-27b | 128 | 3 | 3 | 0 | 530 | 128 | 100 | 19 |
| qwen/qwen3.6-27b | 256 | 3 | 3 | 0 | 781 | 256 | 100 | 96 |
| qwen/qwen3.6-27b | 384 | 3 | 3 | 0 | 989 | 384 | 100 | 174 |
| qwen/qwen3.6-27b | 512 | 3 | 3 | 3 | 1275 | 512 | 100 | 300 |
| qwen/qwen3.6-27b | 768 | 3 | 3 | 3 | 1789 | 768 | 100 | 510 |
| qwen/qwen3.6-27b | 1024 | 3 | 2 | 0 | 1478 | 683 | 67 | 460 |
| qwen/qwen3.6-27b | 2048 | 3 | 1 | 1 | 966 | 402 | 33 | 272 |
