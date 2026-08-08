# Phase 387 — CoT Anti-Poisoning Guard Campaign

**Date:** 2026-08-08 07:43 +0000
**Total trials:** 45

| Model | Provider | Arm | N | Fmt OK | Sem OK | 429 | Err | P50 ms |
|-------|----------|-----|---|--------|--------|-----|-----|--------|
| qwen/qwen3.6-27b | groq | baseline | 5 | 5 | 0 | 0 | 0 | 456 |
| qwen/qwen3.6-27b | groq | format_example | 5 | 5 | 0 | 0 | 0 | 439 |
| qwen/qwen3.6-27b | groq | anti_poison_guard | 5 | 5 | 0 | 0 | 0 | 441 |
| llama-3.3-70b-versatile | groq | baseline | 5 | 5 | 5 | 0 | 0 | 229 |
| llama-3.3-70b-versatile | groq | format_example | 5 | 5 | 5 | 0 | 0 | 311 |
| llama-3.3-70b-versatile | groq | anti_poison_guard | 5 | 5 | 5 | 0 | 0 | 301 |
| deepseek-ai/deepseek-v4-flash-0731 | nim | baseline | 5 | 5 | 5 | 0 | 0 | 759 |
| deepseek-ai/deepseek-v4-flash-0731 | nim | format_example | 5 | 5 | 5 | 0 | 0 | 564 |
| deepseek-ai/deepseek-v4-flash-0731 | nim | anti_poison_guard | 5 | 5 | 5 | 0 | 0 | 3027 |
