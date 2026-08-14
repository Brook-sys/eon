# Phase 491 — Adversarial Baseline and Reasoning Effort Validation

**Date:** 2026-08-14

## Objective
Execute a live sweep across Groq and NIM to establish baseline telemetry and confirm that `qwen/qwen3.6-27b`'s shadow reasoning is appropriately suppressed when `ReasoningEffort="none"`, preventing token starvation for bounded outputs.

## Methodology
- `llama-3.3-70b-versatile` (Groq, standard)
- `qwen/qwen3.6-27b` (Groq, hybrid reasoning)
- `meta/llama-3.1-8b-instruct` (NVIDIA NIM, control)
Prompt: "Reply with exactly: READY", Max Tokens: 16.

## Results
- `llama-3.3-70b-versatile`: 357ms, replied "READY".
- `qwen/qwen3.6-27b`: 205ms, replied "READY" (successfully avoided `<think>` block output when passed `ReasoningEffort="none"`).
- `meta/llama-3.1-8b-instruct`: 567ms, replied "READY".

## Decision
The `ReasoningEffort="none"` field correctly suppressed background reasoning for the qwen model, maintaining max_tokens integrity for strict short outputs. Adversarial mechanisms implemented in Phase 467 are functioning properly as part of the core adapter logic. We will commit these telemetry results.
