# Phase 397 — Reasoning Parameter Recovery & Parser Fire Campaign Report

**Date:** 2026-08-08 17:28:29 +0000
**Total Trials:** 15
**Success Count:** 7 (46.7%)
**P50 Latency:** 512 ms
**P95 Latency:** 2984 ms
**Average Compliance Score:** 0.53

## Strategy Distribution
- **none**: 5
- **primary_prefix**: 8

## Trials Breakdown

| Provider | Model | Case | Effort | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | reasoning_effort_unsupported_recovery | none | none | 0.00 | false | 716ms |
| groq | llama-3.1-8b-instant | multi_key_hybrid_recovery | low | none | 0.00 | false | 326ms |
| groq | llama-3.1-8b-instant | standard_multi_key_clean |  | primary_prefix | 1.00 | true | 418ms |
| groq | llama-3.3-70b-versatile | reasoning_effort_unsupported_recovery | none | primary_prefix | 1.00 | true | 416ms |
| groq | llama-3.3-70b-versatile | multi_key_hybrid_recovery | low | primary_prefix | 1.00 | false | 776ms |
| groq | llama-3.3-70b-versatile | standard_multi_key_clean |  | primary_prefix | 1.00 | true | 321ms |
| groq | qwen/qwen3.6-27b | reasoning_effort_unsupported_recovery | none | primary_prefix | 1.00 | true | 398ms |
| groq | qwen/qwen3.6-27b | multi_key_hybrid_recovery | low | none | 0.00 | false | 986ms |
| groq | qwen/qwen3.6-27b | standard_multi_key_clean |  | primary_prefix | 1.00 | true | 420ms |
| groq | openai/gpt-oss-20b | reasoning_effort_unsupported_recovery | none |  | 0.00 | false | 518ms |
| groq | openai/gpt-oss-20b | multi_key_hybrid_recovery | low | primary_prefix | 1.00 | true | 353ms |
| groq | openai/gpt-oss-20b | standard_multi_key_clean |  |  | 0.00 | false | 512ms |
| nim | meta/llama-3.1-8b-instruct | reasoning_effort_unsupported_recovery | none | none | 0.00 | false | 2984ms |
| nim | meta/llama-3.1-8b-instruct | multi_key_hybrid_recovery | low | none | 0.00 | false | 1798ms |
| nim | meta/llama-3.1-8b-instruct | standard_multi_key_clean |  | primary_prefix | 1.00 | true | 921ms |
