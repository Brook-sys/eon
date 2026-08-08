# Phase 402 — Adversarial Retest & Hardened Go Parser Campaign Report

**Date:** 2026-08-08T20:45:57Z
**Total Trials:** 20
**Success Count:** 20 (100.0%)
**P50 Latency:** 333 ms
**P95 Latency:** 1417 ms
**Average Compliance Score:** 1.00

## Strategy Distribution
- **primary_prefix**: 20

## Trials Breakdown

| Provider | Model | Case | MaxTokens | Retried | Strategy | Compliance | Semantic | Latency |
|---|---|---|---|---|---|---|---|---|
| groq | llama-3.1-8b-instant | adv-language-degradation | 256 | false | primary_prefix | 1.00 | true | 327ms |
| groq | llama-3.1-8b-instant | adv-budget-starvation | 128 | false | primary_prefix | 1.00 | true | 233ms |
| groq | llama-3.1-8b-instant | adv-conflicting-data | 256 | false | primary_prefix | 1.00 | true | 222ms |
| groq | llama-3.1-8b-instant | adv-context-pollution | 256 | false | primary_prefix | 1.00 | true | 222ms |
| groq | llama-3.3-70b-versatile | adv-language-degradation | 256 | false | primary_prefix | 1.00 | true | 247ms |
| groq | llama-3.3-70b-versatile | adv-budget-starvation | 128 | false | primary_prefix | 1.00 | true | 309ms |
| groq | llama-3.3-70b-versatile | adv-conflicting-data | 256 | false | primary_prefix | 1.00 | true | 333ms |
| groq | llama-3.3-70b-versatile | adv-context-pollution | 256 | false | primary_prefix | 1.00 | true | 272ms |
| groq | qwen/qwen3.6-27b | adv-language-degradation | 256 | false | primary_prefix | 1.00 | true | 250ms |
| groq | qwen/qwen3.6-27b | adv-budget-starvation | 128 | false | primary_prefix | 1.00 | true | 483ms |
| groq | qwen/qwen3.6-27b | adv-conflicting-data | 256 | false | primary_prefix | 1.00 | true | 222ms |
| groq | qwen/qwen3.6-27b | adv-context-pollution | 256 | false | primary_prefix | 1.00 | true | 251ms |
| groq | openai/gpt-oss-20b | adv-language-degradation | 1024 | true | primary_prefix | 1.00 | true | 1417ms |
| groq | openai/gpt-oss-20b | adv-budget-starvation | 512 | true | primary_prefix | 1.00 | true | 879ms |
| groq | openai/gpt-oss-20b | adv-conflicting-data | 1024 | true | primary_prefix | 1.00 | true | 1248ms |
| groq | openai/gpt-oss-20b | adv-context-pollution | 1024 | true | primary_prefix | 1.00 | true | 1392ms |
| nim | meta/llama-3.1-8b-instruct | adv-language-degradation | 256 | false | primary_prefix | 1.00 | true | 683ms |
| nim | meta/llama-3.1-8b-instruct | adv-budget-starvation | 128 | false | primary_prefix | 1.00 | true | 978ms |
| nim | meta/llama-3.1-8b-instruct | adv-conflicting-data | 256 | false | primary_prefix | 1.00 | true | 380ms |
| nim | meta/llama-3.1-8b-instruct | adv-context-pollution | 256 | false | primary_prefix | 1.00 | true | 632ms |
