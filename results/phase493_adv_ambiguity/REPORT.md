# Phase 493 — Adversarial Ambiguity & Context Overload Check

**Date:** 2026-08-14

## Objective
Evaluate model performance against ambiguous context where multiple dates and sources are provided, requiring the model to semantically extract only the target entity ("PRIMARY EXECUTION"). Mistral-large-2 added to NIM coverage.

## Methodology
Context provided three dates (plan, execution, post-mortem). Models were asked for "PRIMARY EXECUTION DATE and SOURCE".
Models tested:
- `llama-3.3-70b-versatile` (Groq, strong)
- `qwen/qwen3.6-27b` (Groq, hybrid reasoning with effort suppressed)
- `meta/llama-3.1-8b-instruct` (NIM, 8B control)
- `mistralai/mistral-large-2-instruct` (NIM, large model test)

## Results
- **llama-3.3-70b-versatile**: 380ms. Extracted SYS-B and 2025-05-12 perfectly.
- **qwen/qwen3.6-27b**: 226ms. Extracted SYS-B and 2025-05-12 perfectly (effort disabled).
- **meta/llama-3.1-8b-instruct**: 627ms. Extracted SYS-B and 2025-05-12 perfectly.
- **mistralai/mistral-large-2-instruct**: Failed with HTTP 404 from NIM (Endpoint deprecation or mistyped model slug).

## Decision
All models correctly parsed the ambiguity. 8B Llama models perform adequately on this simple semantic extraction. The Qwen model maintained the correct schema even with reasoning off. The 404 for Mistral on NIM indicates the integration identifier needs to be updated (NIM documentation requires checking for slug renames).
