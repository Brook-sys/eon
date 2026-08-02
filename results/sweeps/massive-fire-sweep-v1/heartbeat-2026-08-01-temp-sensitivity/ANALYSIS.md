# Sweep Analysis: heartbeat-2026-08-01-temp-sensitivity

## Hypothesis

Slice2 observed that raising temperature did not degrade `llama-3.1-8b-instant`
accuracy on the two discriminating tasks (see sibling ANALYSIS). This bounded
control extends the check to `openai/gpt-oss-20b`: does temperature (0.0 vs
0.7) move exact-line accuracy on `extract-date` and `repair-stale-claim`?

## Setup

- **Models:** groq `openai/gpt-oss-20b` only.
- **Tasks:** extract-date (2 expected fields), detect-conflict (1 field),
  temps 0.0 and 0.7 × 2 reps = 8 calls.
- **Calls:** 8 planned, 8 executed, 0 errors, 0 × 429. p50 454 ms, p95 508 ms,
  max 508 ms. `max_tokens_per_call_actual` = 256. Completion tokens stay
  46–78 across both temperatures — well under budget, no truncation.
- `reasoning_effort` resolved from `REASONING_MODEL_DEFAULTS`
  (`groq:openai/gpt-oss-` → `"low"`); recorded null override.

## Result

| Condition | correct |
|-----------|---------|
| t=0.0, 2 tasks × 2 reps | 4/4 |
| t=0.7, 2 tasks × 2 reps | 4/4 |

Aggregate: **8/8 correct at both temperatures** — stability consistent with
the llama-3.1-8b finding; exact-line scoring on these tasks is not sensitive
to sampling temperature inside the tested band. This supports keeping t=0.0 as
the default canary temperature and treating t>0 strictly as a sensitivity
control, not a quality lever.

## Artifacts

- `trials.json` / `summary.json` — 8 trials.
