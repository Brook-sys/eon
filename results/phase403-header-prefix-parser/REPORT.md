# Phase 403 Live Fire Campaign Report: Header Prefix Key Parsing

**Date:** 2026-08-08T21:24:03Z
**Total Trials:** 15
**Overall Success Rate:** 100.0% (15/15)
**Average Format Compliance:** 1.00
**P50 Latency:** 312 ms
**P95 Latency:** 908 ms

## Model Performance Summary

| Model | Success Rate | Avg Compliance | P50 Latency | P95 Latency |
| --- | --- | --- | --- | --- |
| `meta/llama-3.1-8b-instruct` | 100.0% (3/3) | 1.00 | 528 ms | 908 ms |
| `llama-3.1-8b-instant` | 100.0% (3/3) | 1.00 | 295 ms | 299 ms |
| `llama-3.3-70b-versatile` | 100.0% (3/3) | 1.00 | 274 ms | 380 ms |
| `qwen/qwen3.6-27b` | 100.0% (3/3) | 1.00 | 279 ms | 356 ms |
| `openai/gpt-oss-20b` | 100.0% (3/3) | 1.00 | 312 ms | 325 ms |

## Trial Details

| Model | Case | Status | Strategy | Compliance | Semantic | Latency |
| --- | --- | --- | --- | --- | --- | --- |
| `llama-3.1-8b-instant` | `markdown_header_keys` | 200 | `primary_prefix` | 1.00 | true | 295 ms |
| `llama-3.1-8b-instant` | `header_with_bullets` | 200 | `primary_prefix` | 1.00 | true | 299 ms |
| `llama-3.1-8b-instant` | `bold_header_mix` | 200 | `primary_prefix` | 1.00 | true | 231 ms |
| `llama-3.3-70b-versatile` | `markdown_header_keys` | 200 | `primary_prefix` | 1.00 | true | 274 ms |
| `llama-3.3-70b-versatile` | `header_with_bullets` | 200 | `primary_prefix` | 1.00 | true | 380 ms |
| `llama-3.3-70b-versatile` | `bold_header_mix` | 200 | `primary_prefix` | 1.00 | true | 260 ms |
| `qwen/qwen3.6-27b` | `markdown_header_keys` | 200 | `primary_prefix` | 1.00 | true | 279 ms |
| `qwen/qwen3.6-27b` | `header_with_bullets` | 200 | `primary_prefix` | 1.00 | true | 233 ms |
| `qwen/qwen3.6-27b` | `bold_header_mix` | 200 | `primary_prefix` | 1.00 | true | 356 ms |
| `openai/gpt-oss-20b` | `markdown_header_keys` | 200 | `primary_prefix` | 1.00 | true | 312 ms |
| `openai/gpt-oss-20b` | `header_with_bullets` | 200 | `primary_prefix` | 1.00 | true | 325 ms |
| `openai/gpt-oss-20b` | `bold_header_mix` | 200 | `primary_prefix` | 1.00 | true | 312 ms |
| `meta/llama-3.1-8b-instruct` | `markdown_header_keys` | 200 | `primary_prefix` | 1.00 | true | 908 ms |
| `meta/llama-3.1-8b-instruct` | `header_with_bullets` | 200 | `primary_prefix` | 1.00 | true | 407 ms |
| `meta/llama-3.1-8b-instruct` | `bold_header_mix` | 200 | `primary_prefix` | 1.00 | true | 528 ms |
