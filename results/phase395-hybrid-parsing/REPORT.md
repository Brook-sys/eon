# Phase 395 — Hybrid Parsing Live Campaign Report

**Date:** 2026-08-08 16:05:07 +0000
**Total Trials:** 10
**Success Count:** 8 (80.0%)
**P50 Latency:** 452 ms
**P95 Latency:** 3702 ms
**Average Compliance Score:** 0.80

## Strategy Distribution
- **primary_prefix**: 6
- **hybrid_prefix_positional**: 2

## Trials Breakdown

| Provider | Model | Case | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | standard_prefix | primary_prefix | 1.00 | true | 452ms |
| groq | llama-3.1-8b-instant | hybrid_prefix_bare | primary_prefix | 1.00 | true | 316ms |
| groq | llama-3.3-70b-versatile | standard_prefix | primary_prefix | 1.00 | true | 248ms |
| groq | llama-3.3-70b-versatile | hybrid_prefix_bare | hybrid_prefix_positional | 1.00 | true | 321ms |
| groq | qwen/qwen3.6-27b | standard_prefix | primary_prefix | 1.00 | true | 506ms |
| groq | qwen/qwen3.6-27b | hybrid_prefix_bare | hybrid_prefix_positional | 1.00 | true | 518ms |
| groq | openai/gpt-oss-20b | standard_prefix |  | 0.00 | false | 35ms |
| groq | openai/gpt-oss-20b | hybrid_prefix_bare |  | 0.00 | false | 33ms |
| nim | deepseek-ai/deepseek-v4-flash-0731 | standard_prefix | primary_prefix | 1.00 | true | 2541ms |
| nim | deepseek-ai/deepseek-v4-flash-0731 | hybrid_prefix_bare | primary_prefix | 1.00 | true | 3702ms |
