# Phase 405 Live Fire Campaign Report: Arrow & Assignment Separators

**Date:** 2026-08-08T23:22:29Z
**Total Trials:** 15
**Overall Success Rate:** 100.0% (15/15)
**Average Format Compliance:** 1.00
**P50 Latency:** 415 ms
**P95 Latency:** 897 ms

## Model Performance Summary

| Model | Success Rate | Avg Compliance | P50 Latency | P95 Latency |
| --- | --- | --- | --- | --- |
| `openai/gpt-oss-20b` | 100.0% (3/3) | 1.00 | 369 ms | 484 ms |
| `meta/llama-3.1-8b-instruct` | 100.0% (3/3) | 1.00 | 522 ms | 897 ms |
| `llama-3.1-8b-instant` | 100.0% (3/3) | 1.00 | 305 ms | 415 ms |
| `llama-3.3-70b-versatile` | 100.0% (3/3) | 1.00 | 340 ms | 437 ms |
| `qwen/qwen3.6-27b` | 100.0% (3/3) | 1.00 | 573 ms | 708 ms |

## Trial Details

| Model | Case | Status | Strategy | Compliance | Semantic | Latency |
| --- | --- | --- | --- | --- | --- | --- |
| `llama-3.1-8b-instant` | `arrow_separators` | 200 | `primary_prefix` | 1.00 | true | 415 ms |
| `llama-3.1-8b-instant` | `assignment_separators` | 200 | `primary_prefix` | 1.00 | true | 305 ms |
| `llama-3.1-8b-instant` | `bulleted_arrow_mix` | 200 | `primary_prefix` | 1.00 | true | 232 ms |
| `llama-3.3-70b-versatile` | `arrow_separators` | 200 | `primary_prefix` | 1.00 | true | 241 ms |
| `llama-3.3-70b-versatile` | `assignment_separators` | 200 | `primary_prefix` | 1.00 | true | 437 ms |
| `llama-3.3-70b-versatile` | `bulleted_arrow_mix` | 200 | `primary_prefix` | 1.00 | true | 340 ms |
| `qwen/qwen3.6-27b` | `arrow_separators` | 200 | `primary_prefix` | 1.00 | true | 573 ms |
| `qwen/qwen3.6-27b` | `assignment_separators` | 200 | `primary_prefix` | 1.00 | true | 361 ms |
| `qwen/qwen3.6-27b` | `bulleted_arrow_mix` | 200 | `primary_prefix` | 1.00 | true | 708 ms |
| `openai/gpt-oss-20b` | `arrow_separators` | 200 | `primary_prefix` | 1.00 | true | 369 ms |
| `openai/gpt-oss-20b` | `assignment_separators` | 200 | `primary_prefix` | 1.00 | true | 484 ms |
| `openai/gpt-oss-20b` | `bulleted_arrow_mix` | 200 | `primary_prefix` | 1.00 | true | 267 ms |
| `meta/llama-3.1-8b-instruct` | `arrow_separators` | 200 | `primary_prefix` | 1.00 | true | 897 ms |
| `meta/llama-3.1-8b-instruct` | `assignment_separators` | 200 | `primary_prefix` | 1.00 | true | 522 ms |
| `meta/llama-3.1-8b-instruct` | `bulleted_arrow_mix` | 200 | `primary_prefix` | 1.00 | true | 433 ms |
