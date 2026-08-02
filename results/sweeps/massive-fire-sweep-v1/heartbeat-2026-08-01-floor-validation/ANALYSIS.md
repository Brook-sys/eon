# Sweep Analysis: heartbeat-2026-08-01-floor-validation

## Hypothesis

The 256-token floor (vs 128 in `heartbeat-2026-08-01-repair-extract`) closes
the gpt-oss-20b truncation cluster on `extract-date-reinforced` (3/3 fails,
all `finish_reason=length` with partial first lines there), and the reinforced
prompt holds for the NIM 8B control under the new runner path
(`reasoning_effort` defaults now active).

## Setup

- **Models:** groq `openai/gpt-oss-20b` (fix validation;
  `reasoning_effort=low` sent per `REASONING_MODEL_DEFAULTS`),
  nvidia_nim `meta/llama-3.1-8b-instruct` (cross-provider control; no effort
  field — no prefix match).
- **Task:** extract-date-reinforced, t=0.0, 3 reps → **6 calls**, 0 errors,
  0 × 429. `max_tokens_per_call_actual` = 256.
- Executed in heartbeat 2026-08-01 21:20–21:40 (-03) as this cycle's mandatory
  fresh live evidence.

## Result

| Model | correct | finish | completion tokens |
|-------|---------|--------|-------------------|
| groq openai/gpt-oss-20b | 3/3 | stop ×3 | 48, 48, 48 |
| nvidia_nim llama-3.1-8b-instruct | 3/3 | stop ×3 | 16, 16, 16 |

gpt-oss-20b completes with ~19% of the budget used and byte-exact output at
every rep — the truncation failure is decisively closed by the floor, at no
observed cost penalty (48 vs 110 tokens under the repair prompt; effort="low"
keeps thinking off the wire). The NIM control confirms the reinforced prompt
formulation transfers cross-provider at 8B scale with 6.2% of budget.

Convergence note: gpt-oss-20b is now deterministic across reps on this task
(3 identical outputs), matching llama-3.3-70b and the NIM 8B. The remaining
discriminating surface for weak models is the label/value transposition and
`O-` prefix stripping cataloged in the reasoning-effort slice ANALYSIS.

## Artifacts

- `trials.json` / `summary.json` — 6 trials.
