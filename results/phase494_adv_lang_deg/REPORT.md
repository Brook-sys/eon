# Phase 494 — Adversarial Language Degradation Check

**Date:** 2026-08-14

## Objective
Verify format constraint integrity when instructions and output schema are English, but the context is PT-BR with heavy slang. Smaller models historically struggle to separate schema from context semantics here, bleeding the context language into the structural keys.

## Methodology
Context: "O sistema pifou geral ontem de noite, a gambiarra que o Zé fez no banco de dados não segurou a onda e deu timeout na requisição principal..."
Output Schema: `STATUS: <status>` and `REASON: <reason>`
Models: `llama-3.3-70b-versatile`, `llama-3.1-8b-instant`, `qwen/qwen3.6-27b` (reasoning=none), `meta/llama-3.1-8b-instruct`.

## Results
- **llama-3.3-70b-versatile**: 423ms. Clean extraction (`STATUS: Fora do ar`, `REASON: Timeout na requisição...`).
- **llama-3.1-8b-instant (Groq)**: 322ms. Suffered structural confusion (`STATUS: SYSTEM_STATUS: PIFOU`).
- **qwen/qwen3.6-27b**: 218ms. Clean extraction (`STATUS: fora do ar`, `REASON: timeout na...`).
- **meta/llama-3.1-8b-instruct (NIM)**: 696ms. Suffered structural confusion (`STATUS: SYSTEM_STATUS: DOWN`).

## Decision
As identified in Phase 383, the language degradation vector is highly discriminating for 8B models. While they maintain the prefix anchors (`STATUS:`, `REASON:`), they hallucinate the internal task names (`SYSTEM_STATUS:`) inside the value space. Large models (70B, 27B) gracefully translate the slang into concise values while holding the formatting perfectly.
