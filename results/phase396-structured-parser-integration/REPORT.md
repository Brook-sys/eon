# Phase 396 — Structured Parser Integration Live Campaign Report

**Date:** 2026-08-08 16:22:25 +0000
**Total Trials:** 15
**Success Count:** 11 (73.3%)
**P50 Latency:** 340 ms
**P95 Latency:** 1787 ms
**Average Compliance Score:** 0.73

## Strategy Distribution
- **primary_prefix**: 8
- **none**: 1
- **hybrid_prefix_positional**: 3

## Trials Breakdown

| Provider | Model | Case | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | multi_key_standard | primary_prefix | 1.00 | true | 354ms |
| groq | llama-3.1-8b-instant | multi_key_hybrid_pressure | none | 0.00 | false | 531ms |
| groq | llama-3.1-8b-instant | multi_key_reasoning_pressure | primary_prefix | 1.00 | true | 224ms |
| groq | llama-3.3-70b-versatile | multi_key_standard | primary_prefix | 1.00 | true | 329ms |
| groq | llama-3.3-70b-versatile | multi_key_hybrid_pressure | hybrid_prefix_positional | 1.00 | true | 402ms |
| groq | llama-3.3-70b-versatile | multi_key_reasoning_pressure | primary_prefix | 1.00 | true | 238ms |
| groq | qwen/qwen3.6-27b | multi_key_standard | primary_prefix | 1.00 | true | 339ms |
| groq | qwen/qwen3.6-27b | multi_key_hybrid_pressure | hybrid_prefix_positional | 1.00 | true | 430ms |
| groq | qwen/qwen3.6-27b | multi_key_reasoning_pressure | primary_prefix | 1.00 | true | 340ms |
| groq | openai/gpt-oss-20b | multi_key_standard |  | 0.00 | false | 30ms |
| groq | openai/gpt-oss-20b | multi_key_hybrid_pressure |  | 0.00 | false | 32ms |
| groq | openai/gpt-oss-20b | multi_key_reasoning_pressure |  | 0.00 | false | 30ms |
| nim | deepseek-ai/deepseek-v4-flash-0731 | multi_key_standard | primary_prefix | 1.00 | true | 1787ms |
| nim | deepseek-ai/deepseek-v4-flash-0731 | multi_key_hybrid_pressure | hybrid_prefix_positional | 1.00 | true | 1230ms |
| nim | deepseek-ai/deepseek-v4-flash-0731 | multi_key_reasoning_pressure | primary_prefix | 1.00 | true | 1737ms |
