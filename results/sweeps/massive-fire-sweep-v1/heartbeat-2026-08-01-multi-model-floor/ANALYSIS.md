# Sweep Analysis: heartbeat-2026-08-01-multi-model-floor

## Hypothesis

Does the reinforced prompt formulation plus the raised `max_tokens` floor
(manifest default 256, up from the 128 used in `heartbeat-2026-08-01-repair-extract`,
where gpt-oss-20b truncated) generalize across the full provider × model matrix,
including providers/models with no observed truncation history?

## Setup

- **Models (4):** groq `llama-3.3-70b-versatile`, groq `allam-2-7b`,
  groq `qwen/qwen3.6-27b`, nvidia_nim `meta/llama-3.1-8b-instruct`.
- **Tasks:** repair-stale-claim, extract-date-reinforced; t=0.0; 3 reps →
  6 calls/model, **24 calls total**, 0 errors, 0 × 429.
- `max_tokens_per_call_actual` = 256. Latency p50 656 ms, p95 922 ms,
  max 923 ms. `reasoning_effort` override none.

## Result

| Model | correct | failure mode |
|-------|---------|--------------|
| groq llama-3.3-70b-versatile | 6/6 | — |
| groq allam-2-7b | 6/6 | — |
| nvidia_nim llama-3.1-8b-instruct | 6/6 | — |
| groq qwen/qwen3.6-27b | **0/6** | `finish_reason=length`, 256-token cap burned entirely by raw `<think>` text; zero extraction output |

Aggregate: 18/24. The token floor fully resolves the gpt-oss-20b class of
truncation seen in the preceding slice, and the reinforced prompt is stable
across the other three models. The sole failure cluster is qwen3.6-27b
emitting raw thinking *into `content`* until exhaustion of the budget — the
mechanism isolated by `probe_qwen_thinking.py` and closed in the sibling slice
`heartbeat-2026-08-01-reasoning-effort` via `reasoning_effort="none"`
(15/15 rescue). Notably, qwen3.6-27b records in this slice carry no
`reasoning_effort` field because the defaults map did not yet exist when the
slice ran; the regression coverage in `test_runner.py` binds the
`groq:qwen/` → `"none"` resolution so this path is now exercised in
subsequent Groq sweeps.

## Decision

Adopt 256 as the standing per-call budget floor for exact-line tasks
(≥6 tokens/expected field; enforced in the runner) and route all subsequent
qwen3.x Groq calls through the reasoning-effort default. No model-routing
change: qwen3.6-27b is admissible under the effort knob, not by budget growth.

## Artifacts

- `trials.json` / `summary.json` — 24 trials.
