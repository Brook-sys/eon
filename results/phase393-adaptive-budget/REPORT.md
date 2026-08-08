# Phase 393 — Adaptive Reasoning Effort & Multi-Provider Budget Recovery Campaign

**Date:** 2026-08-08 17:30 -03
**Objective:** Evaluate adaptive reasoning effort suppression, BudgetGuard floor validation, and multi-provider cross-model behavior across 5 models under 3 budget regimes (64, 256, 512 max output tokens).

---

## 1. Experimental Setup

- **Providers & Models:**
  - `groq/qwen/qwen3.6-27b` (Reasoning model, `ThinkingOverheadTokens: 640`)
  - `groq/llama-3.3-70b-versatile` (Standard model, `ThinkingOverheadTokens: 0`)
  - `groq/openai/gpt-oss-20b` (Reasoning-capable model, `ThinkingOverheadTokens: 128`)
  - `groq/llama-3.1-8b-instant` (Standard model, `ThinkingOverheadTokens: 0`)
  - `nim/deepseek-ai/deepseek-v4-flash-0731` (NVIDIA NIM control, `ThinkingOverheadTokens: 0`)
- **Scenarios:**
  - `tight-64-simple` (`max_tokens: 64`, budget floor = 25 tokens)
  - `moderate-256-simple` (`max_tokens: 256`)
  - `comfortable-512-simple` (`max_tokens: 512`)
- **Trials:** 5 models × 3 scenarios × 3 reps = **45 live calls**

---

## 2. Key Empirical Findings

1. **Qwen 3.6 27B Reasoning Effort Auto-Suppression (100% Success):**
   - Under `max_tokens: 64`, `ThinkingOverheadTokens` (640) + `minOutputBase` (25) = 665 tokens, which exceeds 64. BudgetGuard automatically suppressed `reasoning_effort` to `"none"`.
   - Result: 9/9 (100%) success across all budgets (64, 256, 512 tokens), P50 latency **248ms**, output tokens exact (19 tokens), `finish_reason=stop`.

2. **Llama 3.3 70B Baseline Adherence (100% Success):**
   - 9/9 (100%) format and semantic success in **247–329ms**, using 15 output tokens per response. Zero error/fallback across all token budgets.

3. **GPT-OSS 20B Reasoning Budget Exhaustion & Auto-Recovery:**
   - At `max_tokens: 64`, `reasoning_effort: "medium"` caused the model to spend all 64 tokens on reasoning, producing zero assistant text content (`finish_reason=length`).
   - The provider adapter classified this accurately as `INVALID_RESPONSE: reasoning_budget_exhausted` (3/3 calls).
   - At `max_tokens: 256` and `512`, `gpt-oss-20b` achieved 6/6 (100%) success in 343–463ms, using 114–122 output tokens.

4. **Llama 3.1 8B Format Pressure Boundary:**
   - At `max_tokens: 64`, the 8B model emitted verbose preamble before the structured answer, hitting `finish_reason=length` (0/3 semantic).
   - At `max_tokens: 256` and `512`, format compliance was 6/6 (100%) in 324–369ms (80 output tokens).

5. **NVIDIA NIM DeepSeek V4 Flash Cross-Provider Control (100% Success):**
   - Achieved 9/9 (100%) format and semantic accuracy across all budget levels (64, 256, 512 tokens).
   - P50 latency ~3.8s (higher tail due to NIM network route), output token count exact (15 tokens).

---

## 3. Campaign Summary Statistics

- **Total Calls:** 45
- **Semantic Success:** 39 / 45 (**86.7%**)
- **Target Scores:**
  - `qwen3.6-27b`: 9/9 (100%) — *Auto-suppression validated*
  - `llama3.3-70b`: 9/9 (100%) — *Baseline control*
  - `nim-deepseek-v4`: 9/9 (100%) — *Cross-provider control*
  - `llama3.1-8b`: 6/9 (66.7%) — *Failed only at tight 64 budget*
  - `gpt-oss-20b`: 6/9 (66.7%) — *Reasoning-eaten budget at 64 tokens classified cleanly*

---

## 4. Architectural Conclusions

- `BudgetGuard` reasoning effort auto-suppression works flawlessly in production.
- `reasoning_budget_exhausted` error classification allows clean differentiation between provider errors, empty content, and token starvation.
- All artifacts saved in `results/phase393-adaptive-budget/`.
