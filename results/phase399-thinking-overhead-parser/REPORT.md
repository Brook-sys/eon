# Phase 399 — Thinking Overhead & Bold Parser Fire Campaign Report

**Date:** 2026-08-08T19:28:29Z
**Total Trials:** 15
**Success Count:** 11 (73.3%)
**P50 Latency:** 386 ms
**P95 Latency:** 735 ms
**Average Compliance Score:** 0.93

## Strategy Distribution
- **primary_prefix**: 11
- **positional_fallback**: 3

## Trials Breakdown

| Provider | Model | Case | Overhead | Suppressed | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | bold_markdown_key_parsing | 0 | false | primary_prefix | 1.00 | true | 334ms |
| groq | llama-3.1-8b-instant | bulleted_bold_key_parsing | 0 | false | positional_fallback | 1.00 | false | 289ms |
| groq | llama-3.1-8b-instant | thinking_overhead_auto_suppress | 0 | false | primary_prefix | 1.00 | true | 278ms |
| groq | llama-3.3-70b-versatile | bold_markdown_key_parsing | 0 | false | primary_prefix | 1.00 | true | 312ms |
| groq | llama-3.3-70b-versatile | bulleted_bold_key_parsing | 0 | false | primary_prefix | 1.00 | true | 299ms |
| groq | llama-3.3-70b-versatile | thinking_overhead_auto_suppress | 0 | false | primary_prefix | 1.00 | true | 245ms |
| groq | qwen/qwen3.6-27b | bold_markdown_key_parsing | 640 | true | primary_prefix | 1.00 | true | 406ms |
| groq | qwen/qwen3.6-27b | bulleted_bold_key_parsing | 640 | true | primary_prefix | 1.00 | true | 496ms |
| groq | qwen/qwen3.6-27b | thinking_overhead_auto_suppress | 640 | true | primary_prefix | 1.00 | true | 274ms |
| groq | openai/gpt-oss-20b | bold_markdown_key_parsing | 256 | true | primary_prefix | 1.00 | true | 504ms |
| groq | openai/gpt-oss-20b | bulleted_bold_key_parsing | 256 | true |  | 0.00 | false | 503ms |
| groq | openai/gpt-oss-20b | thinking_overhead_auto_suppress | 256 | false | positional_fallback | 1.00 | false | 421ms |
| nim | meta/llama-3.1-8b-instruct | bold_markdown_key_parsing | 0 | false | primary_prefix | 1.00 | true | 735ms |
| nim | meta/llama-3.1-8b-instruct | bulleted_bold_key_parsing | 0 | false | positional_fallback | 1.00 | false | 386ms |
| nim | meta/llama-3.1-8b-instruct | thinking_overhead_auto_suppress | 0 | false | primary_prefix | 1.00 | true | 513ms |
