# Phase 397 — Reasoning Parameter Recovery & Parser Fire Campaign Report

**Date:** 2026-08-13 00:42:06 +0000
**Total Trials:** 15
**Success Count:** 8 (53.3%)
**P50 Latency:** 521 ms
**P95 Latency:** 2212 ms
**Average Compliance Score:** 0.73

## Strategy Distribution
- **none**: 2
- **primary_prefix**: 11

## Trials Breakdown

| Provider | Model | Case | Effort | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | reasoning_effort_unsupported_recovery | none | none | 0.00 | false | 613ms |
| groq | llama-3.1-8b-instant | multi_key_hybrid_recovery | low | primary_prefix | 1.00 | false | 375ms |
| groq | llama-3.1-8b-instant | standard_multi_key_clean |  | primary_prefix | 1.00 | true | 521ms |
| groq | llama-3.3-70b-versatile | reasoning_effort_unsupported_recovery | none | primary_prefix | 1.00 | true | 288ms |
| groq | llama-3.3-70b-versatile | multi_key_hybrid_recovery | low | primary_prefix | 1.00 | false | 357ms |
| groq | llama-3.3-70b-versatile | standard_multi_key_clean |  | primary_prefix | 1.00 | true | 251ms |
| groq | qwen/qwen3.6-27b | reasoning_effort_unsupported_recovery | none | primary_prefix | 1.00 | true | 278ms |
| groq | qwen/qwen3.6-27b | multi_key_hybrid_recovery | low | none | 0.00 | false | 723ms |
| groq | qwen/qwen3.6-27b | standard_multi_key_clean |  | primary_prefix | 1.00 | true | 2212ms |
| groq | openai/gpt-oss-20b | reasoning_effort_unsupported_recovery | none |  | 0.00 | false | 586ms |
| groq | openai/gpt-oss-20b | multi_key_hybrid_recovery | low | primary_prefix | 1.00 | true | 342ms |
| groq | openai/gpt-oss-20b | standard_multi_key_clean |  |  | 0.00 | false | 578ms |
| nim | meta/llama-3.1-8b-instruct | reasoning_effort_unsupported_recovery | none | primary_prefix | 1.00 | true | 1091ms |
| nim | meta/llama-3.1-8b-instruct | multi_key_hybrid_recovery | low | primary_prefix | 1.00 | false | 1413ms |
| nim | meta/llama-3.1-8b-instruct | standard_multi_key_clean |  | primary_prefix | 1.00 | true | 506ms |
