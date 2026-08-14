# Phase 387 — CoT Anti-Poisoning Guard Campaign

**Date:** 2026-08-14 05:44 +0000
**Total trials:** 45

| Model | Provider | Arm | N | Fmt OK | Sem OK | 429 | Err | P50 ms |
|-------|----------|-----|---|--------|--------|-----|-----|--------|
| qwen/qwen3.6-27b | groq | baseline | 5 | 5 | 0 | 0 | 0 | 983 |
| qwen/qwen3.6-27b | groq | format_example | 5 | 5 | 0 | 0 | 0 | 448 |
| qwen/qwen3.6-27b | groq | anti_poison_guard | 5 | 5 | 0 | 0 | 0 | 702 |
| llama-3.3-70b-versatile | groq | baseline | 5 | 5 | 5 | 0 | 0 | 304 |
| llama-3.3-70b-versatile | groq | format_example | 5 | 5 | 5 | 0 | 0 | 246 |
| llama-3.3-70b-versatile | groq | anti_poison_guard | 5 | 5 | 5 | 0 | 0 | 230 |
| deepseek-ai/deepseek-v4-flash-0731 | nim | baseline | 5 | 5 | 5 | 0 | 0 | 3411 |
| deepseek-ai/deepseek-v4-flash-0731 | nim | format_example | 5 | 5 | 5 | 0 | 0 | 7796 |
| deepseek-ai/deepseek-v4-flash-0731 | nim | anti_poison_guard | 5 | 5 | 5 | 0 | 0 | 5807 |
