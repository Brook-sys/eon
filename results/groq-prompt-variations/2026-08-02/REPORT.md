# Phase 371 B — llama-3.1-8b-instant prompt variations (2026-08-02 17:40 -03)

## Bounds (declared before execution)

- Model under test: `llama-3.1-8b-instant` on Groq Chat Completions.
- Variants: 4 prompts on the same `overview-empty` empty-state micro-copy contract (same as Phase 370 baseline). `max_tokens=220`, `temperature=0.9`, 5 trials/variant, deadline 60 s/call.
- Early-stop: 2 consecutive 429 → abort. Total cap: 20 calls. Read-only, no canonical promotion.

## Hypothesis

Phase 370 observed 12/15 valid JSON deliveries wrapped in a ` ```json ` fence despite explicit prohibition. Three candidate fixes: (v1) negation-restatement appended at the end, (v2) prefilled assistant opening brace that forces a JSON continuation, (v3) one-shot poison (show the model a bad example with the fence and then ask for a rewrite).

## Evidence

| variant                      | strict valid JSON | fence needed strip | p50   | p95   |
|------------------------------|-------------------:|-------------------:|------:|------:|
| v0 baseline (placebo check)  | 5/5                | 4/5 (80 %)         | 478 ms | 723 ms |
| v1 negation-restatement      | 3/5                | 0/5                | 384 ms | 474 ms |
| v2 prefilled assistant `{`   | 5/5                | 0/5                | 403 ms | 509 ms |
| v3 few-shot poison           | 4/5 (1 HTTP 429)   | 0/5                | 452 ms | 663 ms |

- Baseline reproduces Phase 370 (4/5 fences): the fence-rate is **stable and reproducible**, not a one-off. Placebo check confirms the phenomenon is real.
- v1 eliminated all fences but broke strict JSON validity on 2/5 trials: the extra "MUST end with `}`" instruction made the model emit closing-brace-only fragments when the budget was tight (`output_tokens=48`, `finish=stop`) — the model chose to satisfy the closing brace by truncating content before the JSON was complete. The fence is fixed by trading strict-JSON validity; v1 is a **regression** on the primary metric.
- v2 got 5/5 strict valid, zero fences, without any measurable latency penalty vs baseline (p50 was slightly lower). The prefill forces the first byte to `{`, eliminating the model's option of opening a fence token.
- v3 also eliminated all fences and produced 4/4 valid JSON on the trials that completed; one trial was rejected with HTTP 429 (org TPM ≈ 6000 hit — same wall observed in Phase 371 A at the 2048 step).

## Interpretation

- **Prefilled assistant (`{`) is the strongest leverage**: it eliminates the fence without narrowing the model's output freedom, preserves strict raw-JSON validity, and adds no latency.
- Negation-restatement is a **trap**: it produces surface compliance (zero fences) but degrades semantic integrity by making the model sacrifice body text to satisfy the "must close with `}`" rule when tokens are short. Restating a negation is not free; it competes with the primary contract.
- Few-shot poison works but costs 1 extra assistant turn and one extra user turn; it remains viable when prefill is not available on a provider, and its 429 was provider-side, not a cognitive failure.

## Decision

1. Dashboard-v2 copy generation / any strict raw-JSON contract bound to `llama-3.1-8b-instant` **MUST use prefilled assistant `{`** (v2). v1 (negation-restatement) is explicitly rejected for this model+contract.
2. `Prompt.Compile` in `internal/prompt` needs to expose an optional `PrefillAssistant` flag for contracts that demand strict-object JSON; wiring that into the compiler is queued as the next implementation item (the existing `NegatedListConstraint` helper is the natural place to add a sibling helper).
3. Keep `llama-3.3-70b-versatile` as the primary binding (Phase 370); the 8b-instant becomes a viable **fallback** once the prefill pattern is wired, because 220-token budget is enough when the fence is eliminated mechanically.

## Next experiment

Implement `prompt.PrefillAssistant` + tests in `internal/prompt` (wire into `Compiler` and into the bounded-JSON path used by dashboard-v2 copy-generation call sites), then rerun this same campaign with v2 + strict parser to verify the end-to-end fix.
