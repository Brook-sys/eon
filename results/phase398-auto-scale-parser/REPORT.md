# Phase 398 — Auto-Scale & Bullet Parser Fire Campaign Report

**Date:** 2026-08-08T18:47:20Z
**Total Trials:** 15
**Success Count:** 11 (73.3%)
**P50 Latency:** 420 ms
**P95 Latency:** 1081 ms
**Average Compliance Score:** 0.80

## Strategy Distribution
- **primary_prefix**: 11
- **none**: 2
- **positional_fallback**: 1

## Trials Breakdown

| Provider | Model | Case | Effort | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | bulleted_markdown_key_parsing |  | primary_prefix | 1.00 | true | 382ms |
| groq | llama-3.1-8b-instant | reasoning_budget_auto_scale_test | none | primary_prefix | 1.00 | true | 308ms |
| groq | llama-3.1-8b-instant | multi_key_hybrid_clean |  | primary_prefix | 1.00 | true | 225ms |
| groq | llama-3.3-70b-versatile | bulleted_markdown_key_parsing |  | primary_prefix | 1.00 | true | 413ms |
| groq | llama-3.3-70b-versatile | reasoning_budget_auto_scale_test | none | primary_prefix | 1.00 | true | 272ms |
| groq | llama-3.3-70b-versatile | multi_key_hybrid_clean |  | primary_prefix | 1.00 | true | 355ms |
| groq | qwen/qwen3.6-27b | bulleted_markdown_key_parsing |  | none | 0.00 | false | 782ms |
| groq | qwen/qwen3.6-27b | reasoning_budget_auto_scale_test | none | primary_prefix | 1.00 | true | 312ms |
| groq | qwen/qwen3.6-27b | multi_key_hybrid_clean |  | none | 0.00 | false | 1081ms |
| groq | openai/gpt-oss-20b | bulleted_markdown_key_parsing |  |  | 0.00 | false | 468ms |
| groq | openai/gpt-oss-20b | reasoning_budget_auto_scale_test | none | positional_fallback | 1.00 | false | 451ms |
| groq | openai/gpt-oss-20b | multi_key_hybrid_clean |  | primary_prefix | 1.00 | true | 420ms |
| nim | meta/llama-3.1-8b-instruct | bulleted_markdown_key_parsing |  | primary_prefix | 1.00 | true | 753ms |
| nim | meta/llama-3.1-8b-instruct | reasoning_budget_auto_scale_test | none | primary_prefix | 1.00 | true | 834ms |
| nim | meta/llama-3.1-8b-instruct | multi_key_hybrid_clean |  | primary_prefix | 1.00 | true | 458ms |
