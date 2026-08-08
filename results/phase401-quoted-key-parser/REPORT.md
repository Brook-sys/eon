# Phase 401 — Quoted Key Parser & Multi-Provider Live Fire Campaign Report

**Date:** 2026-08-08T20:22:37Z
**Total Trials:** 15
**Success Count:** 14 (93.3%)
**P50 Latency:** 518 ms
**P95 Latency:** 1515 ms
**Average Compliance Score:** 1.00

## Strategy Distribution
- **primary_prefix**: 14
- **positional_fallback**: 1

## Trials Breakdown

| Provider | Model | Case | MaxTokens | Retried | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | double_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 306ms |
| groq | llama-3.1-8b-instant | backtick_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 582ms |
| groq | llama-3.1-8b-instant | bulleted_single_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 536ms |
| groq | llama-3.3-70b-versatile | double_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 284ms |
| groq | llama-3.3-70b-versatile | backtick_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 323ms |
| groq | llama-3.3-70b-versatile | bulleted_single_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 332ms |
| groq | qwen/qwen3.6-27b | double_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 231ms |
| groq | qwen/qwen3.6-27b | backtick_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 298ms |
| groq | qwen/qwen3.6-27b | bulleted_single_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 264ms |
| groq | openai/gpt-oss-20b | double_quoted_keys | 1024 | true | positional_fallback | 1.00 | false | 1075ms |
| groq | openai/gpt-oss-20b | backtick_quoted_keys | 1024 | true | primary_prefix | 1.00 | true | 1255ms |
| groq | openai/gpt-oss-20b | bulleted_single_quoted_keys | 1024 | true | primary_prefix | 1.00 | true | 1515ms |
| nim | meta/llama-3.1-8b-instruct | double_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 708ms |
| nim | meta/llama-3.1-8b-instruct | backtick_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 518ms |
| nim | meta/llama-3.1-8b-instruct | bulleted_single_quoted_keys | 256 | false | primary_prefix | 1.00 | true | 552ms |
