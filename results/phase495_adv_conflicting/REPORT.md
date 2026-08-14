# Phase 495 — Adversarial Conflicting Data

**Date:** 2026-08-14

## Objective
Assess if strong negative sentiment surrounding a distractor subject causes the model to incorrectly apply that negation to the target subject during binary classification extraction.

## Methodology
Context heavily emphasized that "User-B" does NOT have rights and that it is forbidden. Embedded inside is the simple fact that "User-A" does have rights. Task: Extract if User-A has rights.
Models: `llama-3.3-70b-versatile`, `llama-3.1-8b-instant`, `qwen/qwen3.6-27b`, `meta/llama-3.1-8b-instruct`.

## Results
- **llama-3.3-70b-versatile**: 288ms. Correctly identified `ADMIN: YES`.
- **llama-3.1-8b-instant (Groq)**: 210ms. Correctly identified `ADMIN: YES`.
- **qwen/qwen3.6-27b**: 198ms. Correctly identified `ADMIN: YES`.
- **meta/llama-3.1-8b-instruct (NIM)**: 571ms. Correctly identified `ADMIN: YES`.

## Decision
No models were distracted by the conflicting data/sentiment around the secondary subject. The semantic logic in the Llama 3 and Qwen architectures is strong enough to map the binary YES/NO specifically to the targeted subject.
