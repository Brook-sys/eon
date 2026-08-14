# Phase 492 — Adversarial CoT Anti-Poisoning Anchor Check

**Date:** 2026-08-14

## Objective
Validate the resilience of Groq and NIM models against "poisoned" instructions where few-shot examples contradict the structural rule (e.g. `DATE: <date>` vs `date => 2024-01-01`).

## Methodology
Prompt forced format `DATE: <date>` and `SOURCE: <source>` but supplied an explicit few-shot example using `date =>` and `source identifier =>`. Tested across:
- `llama-3.3-70b-versatile` (Groq, strong)
- `llama-3.1-8b-instant` (Groq, weak/fast)
- `meta/llama-3.1-8b-instruct` (NIM, cross-provider control)

## Results
- **llama-3.3-70b-versatile**: 400ms. Extracted perfectly with strict formatting (`DATE: 2025-05-12`). Ignored poison.
- **llama-3.1-8b-instant**: 301ms. Extracted perfectly (`DATE: 2025-05-12`). Ignored poison.
- **meta/llama-3.1-8b-instruct (NIM)**: 1393ms. Extracted perfectly (`DATE: 2025-05-12`). Ignored poison.

## Decision
All Llama 3 models across endpoints (8B and 70B) demonstrate robust format anchoring, ignoring misleading few-shot examples when the primary constraint block is unambiguous. Telemetry logged.
