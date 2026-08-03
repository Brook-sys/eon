# Dashboard-v2 copy-quality campaign — 2026-08-02 11:00 UTC-3

## Bounds

- 45 live Groq Chat Completions calls (3 models × 3 empty-state pages × 5 trials), temperature 0.9, `max_tokens=220`, deadline 45 s/call. Read-only aesthetic evaluation — zero canonical state promotion, no secrets logged.

## Hypothesis

Larger models (`llama-3.3-70b-versatile`, `qwen/qwen3.6-27b`) should produce clearer and more honest PT-BR micro-copy for dashboard empty states than `llama-3.1-8b-instant` at the same bounded budget.

## Setup

Pages requested as strict JSON `{title, body, action_hint}`: `overview-empty`, `cortex-empty`, `audit-empty`. Prompt forbids markdown fences and placeholders (`TODO`, `lorem`).

## Evidence

### llama-3.1-8b-instant (15/15 HTTP 200)

- p50 latency 463 ms, p95 624 ms (min 375, max 624).
- All 15 responses parse as JSON (strict raw-JSON decode).
- **12/15 wrap the JSON in a Markdown fence** (` ```json `) despite explicit prompt instruction; stripping the fence is required before schema validation.
- 1/15 contains a placeholder-like recommendation ("consulte a documentação" variants). Honest tone overall.

### qwen/qwen3.6-27b (15/15 HTTP 200, 0/15 valid JSON)

- p50 latency 924 ms, p95 1332 ms.
- **All 15 calls consumed the full 220-token budget with `finish_reason=length` and no JSON content.** Every completion opens with `<think>` deliberation markup; the shadow reasoning never yields to the requested answer under this budget.
- Same class of reasoning-eaten budget failure observed with `openai/gpt-oss-20b` in Phase 369 with `max_tokens=8`; qwen exhibits it even at 220 tokens. Classification: schema-framing failure (model emitted no JSON at all), distinct from malformed-JSON failures at larger budgets.

### llama-3.3-70b-versatile (15/15 HTTP 200)

- p50 538 ms, p95 570 ms (min 429, max 571) — faster tail than 8B despite 8× parameter count.
- 15/15 valid raw JSON, 0 fences, 0 placeholders, honest direct tone.
- Keys {title 39 chars avg, body 1 sentence, action_hint present} all within expected bounds.

## Decision

- Dashboard-v2 copy generation binds to `llama-3.3-70b-versatile` for production and `llama-3.1-8b-instant` as fallback (requires fence-strip post-processing); `qwen/qwen3.6-27b` excluded from bounded JSON contracts ≤512 tokens until its thinking-budget floor is characterized separately.
- The reasoning-eaten-budget failure mode is now classified deterministically at the adapter (`reasoning_budget_exhausted` — Phase 369/370); campaigns targeting qwen or gpt-oss must always request ≥512 completion tokens on Groq to observe semantic content.

## Next experiment

Bounded qwen probe at `max_tokens ∈ {512, 1024, 2048}` measuring the token threshold at which its `<think>` phase yields to JSON content; compare against `llama-3.3-70b-versatile` cost-adjusted quality.

Data: `copy-quality-results.json` (45 trial records, sanitized — no prompt bodies, no credentials).
