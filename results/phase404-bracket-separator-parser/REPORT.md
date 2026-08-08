# Phase 404 Live Fire Campaign Report: Bracket Keys & Alternate Separators

**Date:** 2026-08-08T22:03:01Z
**Total Trials:** 15
**Overall Success Rate:** 100.0% (15/15)
**Average Format Compliance:** 1.00
**P50 Latency:** 317 ms
**P95 Latency:** 1336 ms

## Model Performance Summary

| Model | Success Rate | Avg Compliance | P50 Latency | P95 Latency |
| --- | --- | --- | --- | --- |
| `llama-3.3-70b-versatile` | 100.0% (3/3) | 1.00 | 317 ms | 332 ms |
| `qwen/qwen3.6-27b` | 100.0% (3/3) | 1.00 | 310 ms | 409 ms |
| `openai/gpt-oss-20b` | 100.0% (3/3) | 1.00 | 327 ms | 390 ms |
| `meta/llama-3.1-8b-instruct` | 100.0% (3/3) | 1.00 | 780 ms | 1336 ms |
| `llama-3.1-8b-instant` | 100.0% (3/3) | 1.00 | 313 ms | 315 ms |

## Trial Details

| Model | Case | Status | Strategy | Compliance | Semantic | Latency |
| --- | --- | --- | --- | --- | --- | --- |
| `llama-3.1-8b-instant` | `bracket_keys` | 200 | `primary_prefix` | 1.00 | true | 313 ms |
| `llama-3.1-8b-instant` | `dash_separators` | 200 | `primary_prefix` | 1.00 | true | 236 ms |
| `llama-3.1-8b-instant` | `bracket_tags_and_headers` | 200 | `primary_prefix` | 1.00 | true | 315 ms |
| `llama-3.3-70b-versatile` | `bracket_keys` | 200 | `primary_prefix` | 1.00 | true | 264 ms |
| `llama-3.3-70b-versatile` | `dash_separators` | 200 | `primary_prefix` | 1.00 | true | 317 ms |
| `llama-3.3-70b-versatile` | `bracket_tags_and_headers` | 200 | `primary_prefix` | 1.00 | true | 332 ms |
| `qwen/qwen3.6-27b` | `bracket_keys` | 200 | `primary_prefix` | 1.00 | true | 310 ms |
| `qwen/qwen3.6-27b` | `dash_separators` | 200 | `primary_prefix` | 1.00 | true | 409 ms |
| `qwen/qwen3.6-27b` | `bracket_tags_and_headers` | 200 | `primary_prefix` | 1.00 | true | 248 ms |
| `openai/gpt-oss-20b` | `bracket_keys` | 200 | `primary_prefix` | 1.00 | true | 390 ms |
| `openai/gpt-oss-20b` | `dash_separators` | 200 | `primary_prefix` | 1.00 | true | 327 ms |
| `openai/gpt-oss-20b` | `bracket_tags_and_headers` | 200 | `primary_prefix` | 1.00 | true | 314 ms |
| `meta/llama-3.1-8b-instruct` | `bracket_keys` | 200 | `primary_prefix` | 1.00 | true | 1336 ms |
| `meta/llama-3.1-8b-instruct` | `dash_separators` | 200 | `primary_prefix` | 1.00 | true | 780 ms |
| `meta/llama-3.1-8b-instruct` | `bracket_tags_and_headers` | 200 | `primary_prefix` | 1.00 | true | 681 ms |
