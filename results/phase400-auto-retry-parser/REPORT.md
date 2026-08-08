# Phase 400 — Auto-Retry & Budget Escalation Fire Campaign Report

**Date:** 2026-08-08T19:29:26Z
**Total Trials:** 15
**Success Count:** 11 (73.3%)
**P50 Latency:** 370 ms
**P95 Latency:** 1167 ms
**Average Compliance Score:** 1.00

## Strategy Distribution
- **primary_prefix**: 11
- **positional_fallback**: 4

## Trials Breakdown

| Provider | Model | Case | MaxTokens | Retried | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | bold_markdown_key_parsing | 256 | false | primary_prefix | 1.00 | true | 338ms |
| groq | llama-3.1-8b-instant | bulleted_bold_key_parsing | 256 | false | positional_fallback | 1.00 | false | 309ms |
| groq | llama-3.1-8b-instant | thinking_overhead_auto_suppress | 512 | false | primary_prefix | 1.00 | true | 265ms |
| groq | llama-3.3-70b-versatile | bold_markdown_key_parsing | 256 | false | primary_prefix | 1.00 | true | 251ms |
| groq | llama-3.3-70b-versatile | bulleted_bold_key_parsing | 256 | false | primary_prefix | 1.00 | true | 307ms |
| groq | llama-3.3-70b-versatile | thinking_overhead_auto_suppress | 512 | false | primary_prefix | 1.00 | true | 319ms |
| groq | qwen/qwen3.6-27b | bold_markdown_key_parsing | 256 | false | primary_prefix | 1.00 | true | 434ms |
| groq | qwen/qwen3.6-27b | bulleted_bold_key_parsing | 256 | false | primary_prefix | 1.00 | true | 370ms |
| groq | qwen/qwen3.6-27b | thinking_overhead_auto_suppress | 512 | false | primary_prefix | 1.00 | true | 296ms |
| groq | openai/gpt-oss-20b | bold_markdown_key_parsing | 256 | false | primary_prefix | 1.00 | true | 499ms |
| groq | openai/gpt-oss-20b | bulleted_bold_key_parsing | 1024 | true | positional_fallback | 1.00 | false | 1167ms |
| groq | openai/gpt-oss-20b | thinking_overhead_auto_suppress | 512 | false | positional_fallback | 1.00 | false | 401ms |
| nim | meta/llama-3.1-8b-instruct | bold_markdown_key_parsing | 256 | false | primary_prefix | 1.00 | true | 945ms |
| nim | meta/llama-3.1-8b-instruct | bulleted_bold_key_parsing | 256 | false | positional_fallback | 1.00 | false | 428ms |
| nim | meta/llama-3.1-8b-instruct | thinking_overhead_auto_suppress | 512 | false | primary_prefix | 1.00 | true | 429ms |
