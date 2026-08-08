# Phase 386 — Adversarial FormatExample Sweep

**Date:** 2026-08-08 06:09 +0000
**Total trials:** 432

## GROQ results

| Model | Scenario | Example | N | Fmt OK | Sem OK | 429 | Err | P50 ms |
|-------|----------|---------|---|--------|--------|-----|-----|--------|
| allam-2-7b | adv-ambiguous-instruction | No | 4 | 4 | 0 | 0 | 0 | 366 |
| allam-2-7b | adv-ambiguous-instruction | Yes | 4 | 1 | 1 | 3 | 0 | 363 |
| allam-2-7b | adv-budget-starvation | No | 4 | 4 | 0 | 0 | 0 | 327 |
| allam-2-7b | adv-budget-starvation | Yes | 4 | 4 | 0 | 0 | 0 | 327 |
| allam-2-7b | adv-conflicting-data | No | 4 | 4 | 0 | 0 | 0 | 345 |
| allam-2-7b | adv-conflicting-data | Yes | 4 | 4 | 0 | 0 | 0 | 334 |
| allam-2-7b | adv-context-pollution | No | 4 | 1 | 1 | 3 | 0 | 380 |
| allam-2-7b | adv-context-pollution | Yes | 4 | 3 | 3 | 1 | 0 | 343 |
| allam-2-7b | adv-cot-poisoning | No | 4 | 1 | 1 | 3 | 0 | 338 |
| allam-2-7b | adv-cot-poisoning | Yes | 4 | 2 | 2 | 2 | 0 | 411 |
| allam-2-7b | adv-format-pressure | No | 4 | 4 | 4 | 0 | 0 | 331 |
| allam-2-7b | adv-format-pressure | Yes | 4 | 4 | 0 | 0 | 0 | 325 |
| allam-2-7b | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 331 |
| allam-2-7b | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 331 |
| allam-2-7b | adv-prompt-injection | No | 4 | 2 | 2 | 2 | 0 | 375 |
| allam-2-7b | adv-prompt-injection | Yes | 4 | 3 | 3 | 1 | 0 | 338 |
| llama-3.1-8b-instant | adv-ambiguous-instruction | No | 4 | 0 | 0 | 0 | 0 | 339 |
| llama-3.1-8b-instant | adv-ambiguous-instruction | Yes | 4 | 4 | 4 | 0 | 0 | 337 |
| llama-3.1-8b-instant | adv-budget-starvation | No | 4 | 3 | 3 | 1 | 0 | 309 |
| llama-3.1-8b-instant | adv-budget-starvation | Yes | 4 | 4 | 4 | 0 | 0 | 292 |
| llama-3.1-8b-instant | adv-conflicting-data | No | 4 | 2 | 2 | 2 | 0 | 249 |
| llama-3.1-8b-instant | adv-conflicting-data | Yes | 4 | 1 | 1 | 3 | 0 | 241 |
| llama-3.1-8b-instant | adv-context-pollution | No | 4 | 4 | 4 | 0 | 0 | 244 |
| llama-3.1-8b-instant | adv-context-pollution | Yes | 4 | 4 | 4 | 0 | 0 | 259 |
| llama-3.1-8b-instant | adv-cot-poisoning | No | 4 | 1 | 1 | 3 | 0 | 313 |
| llama-3.1-8b-instant | adv-cot-poisoning | Yes | 4 | 1 | 1 | 3 | 0 | 300 |
| llama-3.1-8b-instant | adv-format-pressure | No | 4 | 4 | 4 | 0 | 0 | 277 |
| llama-3.1-8b-instant | adv-format-pressure | Yes | 4 | 2 | 2 | 2 | 0 | 276 |
| llama-3.1-8b-instant | adv-language-degradation | No | 4 | 4 | 0 | 0 | 0 | 319 |
| llama-3.1-8b-instant | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 308 |
| llama-3.1-8b-instant | adv-prompt-injection | No | 4 | 2 | 2 | 2 | 0 | 228 |
| llama-3.1-8b-instant | adv-prompt-injection | Yes | 4 | 1 | 1 | 3 | 0 | 232 |
| llama-3.3-70b-versatile | adv-ambiguous-instruction | No | 4 | 4 | 4 | 0 | 0 | 256 |
| llama-3.3-70b-versatile | adv-ambiguous-instruction | Yes | 4 | 4 | 4 | 0 | 0 | 312 |
| llama-3.3-70b-versatile | adv-budget-starvation | No | 4 | 1 | 1 | 3 | 0 | 309 |
| llama-3.3-70b-versatile | adv-budget-starvation | Yes | 4 | 2 | 2 | 2 | 0 | 302 |
| llama-3.3-70b-versatile | adv-conflicting-data | No | 4 | 3 | 3 | 1 | 0 | 308 |
| llama-3.3-70b-versatile | adv-conflicting-data | Yes | 4 | 3 | 3 | 1 | 0 | 237 |
| llama-3.3-70b-versatile | adv-context-pollution | No | 4 | 2 | 2 | 2 | 0 | 308 |
| llama-3.3-70b-versatile | adv-context-pollution | Yes | 4 | 2 | 2 | 2 | 0 | 301 |
| llama-3.3-70b-versatile | adv-cot-poisoning | No | 4 | 1 | 1 | 3 | 0 | 311 |
| llama-3.3-70b-versatile | adv-cot-poisoning | Yes | 4 | 1 | 1 | 3 | 0 | 385 |
| llama-3.3-70b-versatile | adv-format-pressure | No | 4 | 4 | 4 | 0 | 0 | 352 |
| llama-3.3-70b-versatile | adv-format-pressure | Yes | 4 | 3 | 3 | 1 | 0 | 337 |
| llama-3.3-70b-versatile | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 308 |
| llama-3.3-70b-versatile | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 310 |
| llama-3.3-70b-versatile | adv-prompt-injection | No | 4 | 1 | 1 | 3 | 0 | 234 |
| llama-3.3-70b-versatile | adv-prompt-injection | Yes | 4 | 2 | 2 | 2 | 0 | 373 |
| openai/gpt-oss-120b | adv-ambiguous-instruction | No | 4 | 2 | 2 | 2 | 0 | 406 |
| openai/gpt-oss-120b | adv-ambiguous-instruction | Yes | 4 | 2 | 2 | 2 | 0 | 485 |
| openai/gpt-oss-120b | adv-budget-starvation | No | 4 | 0 | 0 | 0 | 0 | 307 |
| openai/gpt-oss-120b | adv-budget-starvation | Yes | 4 | 0 | 0 | 0 | 0 | 287 |
| openai/gpt-oss-120b | adv-conflicting-data | No | 4 | 4 | 4 | 0 | 0 | 360 |
| openai/gpt-oss-120b | adv-conflicting-data | Yes | 4 | 4 | 4 | 0 | 0 | 381 |
| openai/gpt-oss-120b | adv-context-pollution | No | 4 | 1 | 1 | 3 | 0 | 404 |
| openai/gpt-oss-120b | adv-context-pollution | Yes | 4 | 4 | 4 | 0 | 0 | 370 |
| openai/gpt-oss-120b | adv-cot-poisoning | No | 4 | 4 | 4 | 0 | 0 | 356 |
| openai/gpt-oss-120b | adv-cot-poisoning | Yes | 4 | 4 | 4 | 0 | 0 | 321 |
| openai/gpt-oss-120b | adv-format-pressure | No | 4 | 0 | 0 | 2 | 0 | 366 |
| openai/gpt-oss-120b | adv-format-pressure | Yes | 4 | 0 | 0 | 0 | 0 | 338 |
| openai/gpt-oss-120b | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 433 |
| openai/gpt-oss-120b | adv-language-degradation | Yes | 4 | 1 | 1 | 3 | 0 | 348 |
| openai/gpt-oss-120b | adv-prompt-injection | No | 4 | 4 | 4 | 0 | 0 | 386 |
| openai/gpt-oss-120b | adv-prompt-injection | Yes | 4 | 4 | 4 | 0 | 0 | 407 |
| openai/gpt-oss-20b | adv-ambiguous-instruction | No | 4 | 4 | 4 | 0 | 0 | 312 |
| openai/gpt-oss-20b | adv-ambiguous-instruction | Yes | 4 | 4 | 4 | 0 | 0 | 328 |
| openai/gpt-oss-20b | adv-budget-starvation | No | 4 | 0 | 0 | 0 | 0 | 255 |
| openai/gpt-oss-20b | adv-budget-starvation | Yes | 4 | 0 | 0 | 0 | 0 | 250 |
| openai/gpt-oss-20b | adv-conflicting-data | No | 4 | 2 | 2 | 2 | 0 | 347 |
| openai/gpt-oss-20b | adv-conflicting-data | Yes | 4 | 1 | 1 | 3 | 0 | 336 |
| openai/gpt-oss-20b | adv-context-pollution | No | 4 | 4 | 4 | 0 | 0 | 289 |
| openai/gpt-oss-20b | adv-context-pollution | Yes | 4 | 1 | 1 | 3 | 0 | 300 |
| openai/gpt-oss-20b | adv-cot-poisoning | No | 4 | 4 | 4 | 0 | 0 | 339 |
| openai/gpt-oss-20b | adv-cot-poisoning | Yes | 4 | 4 | 4 | 0 | 0 | 273 |
| openai/gpt-oss-20b | adv-format-pressure | No | 4 | 0 | 0 | 0 | 0 | 280 |
| openai/gpt-oss-20b | adv-format-pressure | Yes | 4 | 4 | 4 | 0 | 0 | 290 |
| openai/gpt-oss-20b | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 340 |
| openai/gpt-oss-20b | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 269 |
| openai/gpt-oss-20b | adv-prompt-injection | No | 4 | 1 | 1 | 3 | 0 | 245 |
| openai/gpt-oss-20b | adv-prompt-injection | Yes | 4 | 3 | 3 | 1 | 0 | 309 |
| qwen/qwen3.6-27b | adv-ambiguous-instruction | No | 4 | 4 | 4 | 0 | 0 | 245 |
| qwen/qwen3.6-27b | adv-ambiguous-instruction | Yes | 4 | 4 | 4 | 0 | 0 | 253 |
| qwen/qwen3.6-27b | adv-budget-starvation | No | 4 | 2 | 2 | 2 | 0 | 227 |
| qwen/qwen3.6-27b | adv-budget-starvation | Yes | 4 | 4 | 4 | 0 | 0 | 233 |
| qwen/qwen3.6-27b | adv-conflicting-data | No | 4 | 4 | 4 | 0 | 0 | 252 |
| qwen/qwen3.6-27b | adv-conflicting-data | Yes | 4 | 4 | 4 | 0 | 0 | 255 |
| qwen/qwen3.6-27b | adv-context-pollution | No | 4 | 4 | 4 | 0 | 0 | 391 |
| qwen/qwen3.6-27b | adv-context-pollution | Yes | 4 | 4 | 4 | 0 | 0 | 256 |
| qwen/qwen3.6-27b | adv-cot-poisoning | No | 4 | 1 | 1 | 3 | 0 | 262 |
| qwen/qwen3.6-27b | adv-cot-poisoning | Yes | 4 | 1 | 1 | 3 | 0 | 254 |
| qwen/qwen3.6-27b | adv-format-pressure | No | 4 | 4 | 4 | 0 | 0 | 390 |
| qwen/qwen3.6-27b | adv-format-pressure | Yes | 4 | 4 | 4 | 0 | 0 | 510 |
| qwen/qwen3.6-27b | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 242 |
| qwen/qwen3.6-27b | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 238 |
| qwen/qwen3.6-27b | adv-prompt-injection | No | 4 | 1 | 1 | 3 | 0 | 258 |
| qwen/qwen3.6-27b | adv-prompt-injection | Yes | 4 | 1 | 1 | 3 | 0 | 515 |

## NIM results

| Model | Scenario | Example | N | Fmt OK | Sem OK | 429 | Err | P50 ms |
|-------|----------|---------|---|--------|--------|-----|-----|--------|
| deepseek-ai/deepseek-v4-flash-0731 | adv-ambiguous-instruction | Yes | 3 | 3 | 3 | 0 | 0 | 582 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-budget-starvation | Yes | 3 | 3 | 3 | 0 | 0 | 496 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-conflicting-data | Yes | 3 | 3 | 3 | 0 | 0 | 2598 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-context-pollution | Yes | 3 | 3 | 3 | 0 | 0 | 751 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-cot-poisoning | Yes | 3 | 3 | 3 | 0 | 0 | 422 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-format-pressure | Yes | 3 | 3 | 3 | 0 | 0 | 590 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-language-degradation | Yes | 3 | 3 | 3 | 0 | 0 | 906 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-prompt-injection | Yes | 3 | 3 | 3 | 0 | 0 | 1471 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-ambiguous-instruction | Yes | 3 | 0 | 0 | 0 | 0 | 5885 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-budget-starvation | Yes | 3 | 0 | 0 | 0 | 0 | 1126 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-conflicting-data | Yes | 3 | 0 | 0 | 0 | 0 | 5940 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-context-pollution | Yes | 3 | 0 | 0 | 0 | 0 | 8052 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-cot-poisoning | Yes | 3 | 0 | 0 | 0 | 0 | 5529 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-format-pressure | Yes | 3 | 0 | 0 | 0 | 0 | 2248 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-language-degradation | Yes | 3 | 0 | 0 | 0 | 0 | 5579 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-prompt-injection | Yes | 3 | 0 | 0 | 0 | 0 | 5093 |

## Summary

- **Groq:** 384 calls, 289 OK, 95 429
- **NIM:** 48 calls, 48 OK
- **Latency:** P50=326ms P95=5529ms P99=9007ms

## Key Findings

_(to be filled after analysis)_
