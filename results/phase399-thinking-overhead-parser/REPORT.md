# Phase 399 — Thinking Overhead & Bold Parser Fire Campaign Report

**Date:** 2026-08-14T06:28:22Z
**Total Trials:** 15
**Success Count:** 11 (73.3%)
**P50 Latency:** 241 ms
**P95 Latency:** 647 ms
**Average Compliance Score:** 0.87

## Strategy Distribution
- **positional_fallback**: 2
- **primary_prefix**: 11

## Trials Breakdown

| Provider | Model | Case | Overhead | Suppressed | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | bold_markdown_key_parsing | 0 | false | primary_prefix | 1.00 | true | 291ms |
| groq | llama-3.1-8b-instant | bulleted_bold_key_parsing | 0 | false | positional_fallback | 1.00 | false | 214ms |
| groq | llama-3.1-8b-instant | thinking_overhead_auto_suppress | 0 | false | primary_prefix | 1.00 | true | 207ms |
| groq | llama-3.3-70b-versatile | bold_markdown_key_parsing | 0 | false | primary_prefix | 1.00 | true | 233ms |
| groq | llama-3.3-70b-versatile | bulleted_bold_key_parsing | 0 | false | primary_prefix | 1.00 | true | 217ms |
| groq | llama-3.3-70b-versatile | thinking_overhead_auto_suppress | 0 | false | primary_prefix | 1.00 | true | 319ms |
| groq | qwen/qwen3.6-27b | bold_markdown_key_parsing | 640 | true | primary_prefix | 1.00 | true | 236ms |
| groq | qwen/qwen3.6-27b | bulleted_bold_key_parsing | 640 | true | primary_prefix | 1.00 | true | 241ms |
| groq | qwen/qwen3.6-27b | thinking_overhead_auto_suppress | 640 | true | primary_prefix | 1.00 | true | 224ms |
| groq | openai/gpt-oss-20b | bold_markdown_key_parsing | 256 | true | primary_prefix | 1.00 | true | 472ms |
| groq | openai/gpt-oss-20b | bulleted_bold_key_parsing | 256 | true |  | 0.00 | false | 659ms |
| groq | openai/gpt-oss-20b | thinking_overhead_auto_suppress | 256 | false |  | 0.00 | false | 799ms |
| nim | meta/llama-3.1-8b-instruct | bold_markdown_key_parsing | 0 | false | primary_prefix | 1.00 | true | 647ms |
| nim | meta/llama-3.1-8b-instruct | bulleted_bold_key_parsing | 0 | false | positional_fallback | 1.00 | false | 547ms |
| nim | meta/llama-3.1-8b-instruct | thinking_overhead_auto_suppress | 0 | false | primary_prefix | 1.00 | true | 354ms |
