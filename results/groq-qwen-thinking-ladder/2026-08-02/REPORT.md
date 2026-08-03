# Phase 371 A — Qwen thinking-budget ladder (2026-08-02 17:35 -03)

## Bounds (declared before execution)

- Model under test: `qwen/qwen3.6-27b` on Groq Chat Completions.
- Steps: `max_tokens ∈ {512, 1024, 2048}`, 5 trials/step, `temperature=0.9`, deadline 60 s/call.
- Contract: strict raw JSON `{title, body, action_hint}` PT-BR for the `overview-empty` page (same as Phase 370 copy-quality campaign).
- Early-stop: 2 consecutive HTTP 429 → abort; 5/5 valid at a step → stop ascending.
- Hard ceiling: 15 calls. Read-only, zero canonical state, no credentials in artifacts.

## Hypothesis

Phase 370 showed 0/15 at `max_tokens=220` (consumed the full budget in `<think>` and hit `finish_reason=length`). The thinking-budget floor should be somewhere in the 512–2048 range; locate the first step where the model yields to the requested JSON output.

## Evidence

| max_tokens | valid JSON | finish=length | finish=stop | p50 | p95 | `<think>` opened |
|------------|-----------:|--------------:|------------:|----:|----:|----------------:|
| 512        | 0/5        | 5/5           | 0/5         | 1461 ms | 3476 ms | 5/5 |
| 1024       | 2/5        | 3/5           | 2/5         | 2293 ms | 2419 ms | 5/5 |
| 2048       | (1 trial)  | 1/1           | 0/1         | —    | —    | 1/1 |

- Early-stop fired at `max_tokens=2048` trial 1: two consecutive HTTP 429.
- At 512 tokens the model never closes `<think>`; the entier budget is consumed by deliberation.
- At 1024 the distribution bimodalizes: some completions yield `valid_json` (`output_tokens` 535–785, `finish=stop`), others still exhaust the 1024 budget inside `<think>`.
- `completion_tokens_details.reasoning_tokens` was `null` in all responses (Groq does not return the split for this deployment), so the reasoning/token cut could not be observed directly — only inferred from the presence/absence of `</think>` and `finish_reason`.

## Interpretation

- The thinking-budget floor is **strictly above 512** (0% yield) and **at-most 1024** (40% yield), with high variance at 1024.
- The 429 early-stop at 2048 is consistent with hitting Groq's tokens-per-minute ceiling for this org (Phase 371 B hit the same wall on the `llama-3.1-8b-instant` TPM ≈ 6000 shortly after). This is the provider's safety boundary and was honored: no retries were attempted after the second consecutive 429.
- For contracts that demand strict raw JSON in ≤512 tokens, `qwen/qwen3.6-27b` remains **excluded** (Phase 370 decision confirmed and sharpened: the floor is not at 512, it is between 512 and 1024).

## Decision

1. Keep the dashboard-v2 JSON copy binding on `llama-3.3-70b-versatile`; do not route bounded-JSON contracts to `qwen/qwen3.6-27b` below `max_tokens=1024`.
2. For safety, treat `qwen3.6-27b`'s first-step floor as ≥1024 with ~50 % failure variance; do not hard-code 1024 as a production budget without an additional confirmation batch at n ≥ 10.
3. Do not retry the 2048 step in this rotation window; the TPM error shows the org limit is the active constraint, not the model.

## Next experiment

Single bounded probe at `max_tokens=1024` with n=10 (not n=5) to tighten the variance estimate now that the ladder has bracketed the floor; combine with the structurally-heterogeneous JSON contract from Phase 264 metric (decision/length/concurrency) rather than single-key micro-copy, to see if the floor is contract-shape or contract-length dependent.
