# Phase 388 — Thinking Budget Threshold Campaign

**Date:** 2026-08-08 08:08 +0000
**Total trials:** 42

**Hypothesis:** qwen3.6-27b needs at least 384-512 max_tokens to emit
both thinking tokens and the structured DATE:/SOURCE: answer.
Below that, finish_reason=length truncates before the answer.

**Models:** qwen/qwen3.6-27b (thinking), llama-3.3-70b-versatile (control)

| Model | Max Tok | N | Fmt OK | Sem OK | Avg Lat ms | Avg Comp Tok | Avg Think Tok | Avg Ans Tok |
|-------|---------|---|--------|--------|------------|--------------|---------------|-------------|
| llama-3.3-70b-versatile | 128 | 3 | 3 | 3 | 311 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 256 | 3 | 3 | 3 | 301 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 384 | 3 | 3 | 3 | 293 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 512 | 3 | 3 | 3 | 253 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 768 | 3 | 3 | 3 | 251 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 1024 | 3 | 3 | 3 | 258 | 16 | 0 | 0 |
| llama-3.3-70b-versatile | 2048 | 3 | 3 | 3 | 300 | 16 | 0 | 0 |
| qwen/qwen3.6-27b | 128 | 3 | 3 | 0 | 660 | 128 | 100 | 19 |
| qwen/qwen3.6-27b | 256 | 3 | 3 | 0 | 706 | 256 | 100 | 96 |
| qwen/qwen3.6-27b | 384 | 3 | 3 | 0 | 1045 | 384 | 100 | 174 |
| qwen/qwen3.6-27b | 512 | 3 | 3 | 3 | 1271 | 512 | 100 | 300 |
| qwen/qwen3.6-27b | 768 | 3 | 3 | 3 | 1948 | 768 | 100 | 497 |
| qwen/qwen3.6-27b | 1024 | 3 | 1 | 0 | 803 | 341 | 33 | 230 |
| qwen/qwen3.6-27b | 2048 | 3 | 1 | 1 | 963 | 392 | 33 | 263 |
