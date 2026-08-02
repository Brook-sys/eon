# Sweep Analysis: heartbeat-2026-08-01-reasoning-effort

## Hypothesis

Groq's `reasoning_effort` request parameter (a provider extension on the
OpenAI-compatible surface) disables or downgrades chain-of-thought emission on
hybrid reasoning models, which should (a) fully rescue qwen/qwen3.6-27b from its
0/9 truncation failure observed in slice2 and (b) cut the 8-10x completion-token
overhead observed on openai/gpt-oss-{20b,120b}, without hurting exact-line
accuracy.

## Pre-probe (scripts/sweep/probe_qwen_thinking.py)

Three bounded calls against qwen/qwen3.6-27b on the extract-date prompt
(artifact: `probe_qwen_thinking_result.json`):

| Variant | finish | comp. tokens | content correct |
|---------|--------|--------------|-----------------|
| baseline (temp=0, mt=64) | length | 64 (cap) | 0 — raw `<think>` text floods content |
| reasoning_effort=none, mt=64 | stop | 21 | 1 — exact `DATE: 2025-11-03\nSOURCE: S-17` |
| reasoning_effort=none, mt=256 | stop | 21 | 1 |

Decisive: `reasoning_effort="none"` removes all thinking emission from
`content` on Groq qwen3.x, and no `reasoning_content` field is returned either.
The parameter is therefore sufficient on its own; no `<think>` stripping layer
is needed at the provider boundary for Groq.

## Runner change under test

`runner.py` gained:

- `EXTRA_BODY_ALLOWLIST = {"reasoning_effort"}` — fail-closed allowlist for
  provider-specific request fields; unknown keys are rejected with
  `error_class=config` before any network call.
- `REASONING_MODEL_DEFAULTS` — provider-prefix → effort map
  (`groq:qwen/` → `none`, `groq:openai/gpt-oss-` → `low`), applied per model
  unless `--reasoning-effort` overrides.
- `reasoning_effort_for()` resolver (CLI override wins, else prefix default,
  else no field sent).
- Summary/trials now record the resolved effort per trial for auditability.

Offline regression coverage in `scripts/sweep/test_runner.py` grew from 11 to
16 tests (default resolution, override precedence, allowlist rejection).
All 16 pass.

## Setup

- **Sweep:** massive-fire-sweep-v1, slice `heartbeat-2026-08-01-reasoning-effort`
- **Models:** qwen/qwen3.6-27b (fix validation), openai/gpt-oss-20b,
  openai/gpt-oss-120b (token-overhead check), llama-3.1-8b-instant (control,
  effort not sent — no prefix match)
- **Tasks:** all 5 (extract-date, synthesize-counterexample, detect-conflict,
  repair-stale-claim, extract-date-reinforced), temp=0.0, 3 reps
- **Limits:** max_calls_total=480, per_model=60, concurrency=2, timeout=45s,
  max_tokens=256/call. Actual calls: **60**, zero errors, zero 429s.
- Control slice `heartbeat-2026-08-01-temp-sensitivity`: gpt-oss-20b on the two
  discriminating tasks at t=0.0 and t=0.7 (8 calls) — stable 8/8.

## Results

| Model | Slice2 (no effort param) | This slice (effort default) | Δ |
|-------|--------------------------|-----------------------------|---|
| qwen/qwen3.6-27b | 0/9 (0%), finish=length, 256 tok/call | **15/15 (100%)**, finish=stop | +15 |
| openai/gpt-oss-20b | 9/9, ~138 comp tok avg | **15/15**, 21 tok/call | tokens -85% |
| openai/gpt-oss-120b | 9/9, ~116 comp tok avg | **15/15**, 25 tok/call | tokens -78% |
| llama-3.1-8b-instant | 3/9 | 9/15 (extract-date-reinforced now passes) | control, effort not sent |

Latency p50 = 460 ms, p95 = 996 ms, max = 1.1 s, zero errors — Groq absorbs the
extra request field without penalty.

## Residual failures (llama-3.1-8b-instant, effort-independent)

Two failure modes persist at temp=0, both consistent with slice2:

1. **extract-date:** emits `2025-11-03: S-17` as first line instead of
   `DATE: 2025-11-03` — label/value transposition. The
   `extract-date-reinforced` task (which restates the exact first-line prefix)
   passes 3/3, confirming this is a prompt-formulation sensitivity, not an
   extraction-capability gap.
2. **detect-conflict:** emits `PAIR: 1/2` instead of `PAIR: O-1/O-2` — strips
   the `O-` canonical identifier prefix. Same failure observed on NVIDIA NIM
   llama-3.1-8b-instruct in slice2, i.e. model-family behavior, not provider
   noise.

Both failures are exactly the class the runtime's prompt compiler must absorb
for weak models: restate literal prefixes and canonical identifier shapes in
the constraint block rather than once in the format template.

## Decisions

1. **Adopt reasoning_effort defaults in the sweep runner** (this change) —
   evidence above beats the two alternatives considered in slice2 ANALYSIS
   (`<think>` stripping, bigger budgets): zero code in the hot scoring path,
   deterministic contract preserved, token cost falls ~80% on gpt-oss.
2. **Keep the allowlist fail-closed.** Provider extensions enter the wire
   contract only by explicit edit + test, never implicitly.
3. **Runtime implication (not implemented here):** when the Go
   `ModelProvider` adapter is later asked to support hybrid reasoning models,
   the bounded-budget path should carry an equivalent effort knob per
   deployment profile rather than raise `max_tokens`. The sweep evidence is
   the decision input; adapter changes stay out of this commit per
   separation between harness and runtime.
4. **Prompt-level follow-up queued:** identifier-shape restatement
   (`PAIR must keep the O- prefix verbatim`) for the llama-3.1-8b family,
   alongside the existing extract-date-reinforced pattern. No model routing
   change is warranted: at 9/15 the model remains admissible for reinforced
   tasks only.

## Artifacts

- `heartbeat-2026-08-01-reasoning-effort/{trials,summary}.json` — 60 trials
- `heartbeat-2026-08-01-temp-sensitivity/{trials,summary}.json` — 8 trials
- `scripts/sweep/probe_qwen_thinking.py` — reproducer for the 3-call pre-probe
- `scripts/sweep/probe_qwen_thinking_result.json` — pre-probe artifact
