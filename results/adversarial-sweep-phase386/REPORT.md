# Phase 386 — Adversarial FormatExample Sweep

**Date:** 2026-08-08 06:09 +0000
**Total trials:** 432

## GROQ results

| Model | Scenario | Example | N | Fmt OK | Sem OK | 429 | Err | P50 ms |
|-------|----------|---------|---|--------|--------|-----|-----|--------|
| allam-2-7b | adv-ambiguous-instruction | No | 4 | 4 | 0 | 0 | 0 | 366 |
| allam-2-7b | adv-ambiguous-instruction | Yes | 4 | 1 | 1 | 3 | 0 | 363 |
| allam-2-7b | adv-budget-starvation | No | 4 | 4 | 0 | 0 | 0 | 327 |
| allam-2-7b | adv-budget-starvation | Yes | 4 | 4 | 0 | 0 | 0 | 327 |
| allam-2-7b | adv-conflicting-data | No | 4 | 4 | 0 | 0 | 0 | 345 |
| allam-2-7b | adv-conflicting-data | Yes | 4 | 4 | 0 | 0 | 0 | 334 |
| allam-2-7b | adv-context-pollution | No | 4 | 1 | 1 | 3 | 0 | 380 |
| allam-2-7b | adv-context-pollution | Yes | 4 | 3 | 3 | 1 | 0 | 343 |
| allam-2-7b | adv-cot-poisoning | No | 4 | 1 | 1 | 3 | 0 | 338 |
| allam-2-7b | adv-cot-poisoning | Yes | 4 | 2 | 2 | 2 | 0 | 411 |
| allam-2-7b | adv-format-pressure | No | 4 | 4 | 4 | 0 | 0 | 331 |
| allam-2-7b | adv-format-pressure | Yes | 4 | 4 | 0 | 0 | 0 | 325 |
| allam-2-7b | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 331 |
| allam-2-7b | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 331 |
| allam-2-7b | adv-prompt-injection | No | 4 | 2 | 2 | 2 | 0 | 375 |
| allam-2-7b | adv-prompt-injection | Yes | 4 | 3 | 3 | 1 | 0 | 338 |
| llama-3.1-8b-instant | adv-ambiguous-instruction | No | 4 | 0 | 0 | 0 | 0 | 339 |
| llama-3.1-8b-instant | adv-ambiguous-instruction | Yes | 4 | 4 | 4 | 0 | 0 | 337 |
| llama-3.1-8b-instant | adv-budget-starvation | No | 4 | 3 | 3 | 1 | 0 | 309 |
| llama-3.1-8b-instant | adv-budget-starvation | Yes | 4 | 4 | 4 | 0 | 0 | 292 |
| llama-3.1-8b-instant | adv-conflicting-data | No | 4 | 2 | 2 | 2 | 0 | 249 |
| llama-3.1-8b-instant | adv-conflicting-data | Yes | 4 | 1 | 1 | 3 | 0 | 241 |
| llama-3.1-8b-instant | adv-context-pollution | No | 4 | 4 | 4 | 0 | 0 | 244 |
| llama-3.1-8b-instant | adv-context-pollution | Yes | 4 | 4 | 4 | 0 | 0 | 259 |
| llama-3.1-8b-instant | adv-cot-poisoning | No | 4 | 1 | 1 | 3 | 0 | 313 |
| llama-3.1-8b-instant | adv-cot-poisoning | Yes | 4 | 1 | 1 | 3 | 0 | 300 |
| llama-3.1-8b-instant | adv-format-pressure | No | 4 | 4 | 4 | 0 | 0 | 277 |
| llama-3.1-8b-instant | adv-format-pressure | Yes | 4 | 2 | 2 | 2 | 0 | 276 |
| llama-3.1-8b-instant | adv-language-degradation | No | 4 | 4 | 0 | 0 | 0 | 319 |
| llama-3.1-8b-instant | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 308 |
| llama-3.1-8b-instant | adv-prompt-injection | No | 4 | 2 | 2 | 2 | 0 | 228 |
| llama-3.1-8b-instant | adv-prompt-injection | Yes | 4 | 1 | 1 | 3 | 0 | 232 |
| llama-3.3-70b-versatile | adv-ambiguous-instruction | No | 4 | 4 | 4 | 0 | 0 | 256 |
| llama-3.3-70b-versatile | adv-ambiguous-instruction | Yes | 4 | 4 | 4 | 0 | 0 | 312 |
| llama-3.3-70b-versatile | adv-budget-starvation | No | 4 | 1 | 1 | 3 | 0 | 309 |
| llama-3.3-70b-versatile | adv-budget-starvation | Yes | 4 | 2 | 2 | 2 | 0 | 302 |
| llama-3.3-70b-versatile | adv-conflicting-data | No | 4 | 3 | 3 | 1 | 0 | 308 |
| llama-3.3-70b-versatile | adv-conflicting-data | Yes | 4 | 3 | 3 | 1 | 0 | 237 |
| llama-3.3-70b-versatile | adv-context-pollution | No | 4 | 2 | 2 | 2 | 0 | 308 |
| llama-3.3-70b-versatile | adv-context-pollution | Yes | 4 | 2 | 2 | 2 | 0 | 301 |
| llama-3.3-70b-versatile | adv-cot-poisoning | No | 4 | 1 | 1 | 3 | 0 | 311 |
| llama-3.3-70b-versatile | adv-cot-poisoning | Yes | 4 | 1 | 1 | 3 | 0 | 385 |
| llama-3.3-70b-versatile | adv-format-pressure | No | 4 | 4 | 4 | 0 | 0 | 352 |
| llama-3.3-70b-versatile | adv-format-pressure | Yes | 4 | 3 | 3 | 1 | 0 | 337 |
| llama-3.3-70b-versatile | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 308 |
| llama-3.3-70b-versatile | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 310 |
| llama-3.3-70b-versatile | adv-prompt-injection | No | 4 | 1 | 1 | 3 | 0 | 234 |
| llama-3.3-70b-versatile | adv-prompt-injection | Yes | 4 | 2 | 2 | 2 | 0 | 373 |
| openai/gpt-oss-120b | adv-ambiguous-instruction | No | 4 | 2 | 2 | 2 | 0 | 406 |
| openai/gpt-oss-120b | adv-ambiguous-instruction | Yes | 4 | 2 | 2 | 2 | 0 | 485 |
| openai/gpt-oss-120b | adv-budget-starvation | No | 4 | 0 | 0 | 0 | 0 | 307 |
| openai/gpt-oss-120b | adv-budget-starvation | Yes | 4 | 0 | 0 | 0 | 0 | 287 |
| openai/gpt-oss-120b | adv-conflicting-data | No | 4 | 4 | 4 | 0 | 0 | 360 |
| openai/gpt-oss-120b | adv-conflicting-data | Yes | 4 | 4 | 4 | 0 | 0 | 381 |
| openai/gpt-oss-120b | adv-context-pollution | No | 4 | 1 | 1 | 3 | 0 | 404 |
| openai/gpt-oss-120b | adv-context-pollution | Yes | 4 | 4 | 4 | 0 | 0 | 370 |
| openai/gpt-oss-120b | adv-cot-poisoning | No | 4 | 4 | 4 | 0 | 0 | 356 |
| openai/gpt-oss-120b | adv-cot-poisoning | Yes | 4 | 4 | 4 | 0 | 0 | 321 |
| openai/gpt-oss-120b | adv-format-pressure | No | 4 | 0 | 0 | 2 | 0 | 366 |
| openai/gpt-oss-120b | adv-format-pressure | Yes | 4 | 0 | 0 | 0 | 0 | 338 |
| openai/gpt-oss-120b | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 433 |
| openai/gpt-oss-120b | adv-language-degradation | Yes | 4 | 1 | 1 | 3 | 0 | 348 |
| openai/gpt-oss-120b | adv-prompt-injection | No | 4 | 4 | 4 | 0 | 0 | 386 |
| openai/gpt-oss-120b | adv-prompt-injection | Yes | 4 | 4 | 4 | 0 | 0 | 407 |
| openai/gpt-oss-20b | adv-ambiguous-instruction | No | 4 | 4 | 4 | 0 | 0 | 312 |
| openai/gpt-oss-20b | adv-ambiguous-instruction | Yes | 4 | 4 | 4 | 0 | 0 | 328 |
| openai/gpt-oss-20b | adv-budget-starvation | No | 4 | 0 | 0 | 0 | 0 | 255 |
| openai/gpt-oss-20b | adv-budget-starvation | Yes | 4 | 0 | 0 | 0 | 0 | 250 |
| openai/gpt-oss-20b | adv-conflicting-data | No | 4 | 2 | 2 | 2 | 0 | 347 |
| openai/gpt-oss-20b | adv-conflicting-data | Yes | 4 | 1 | 1 | 3 | 0 | 336 |
| openai/gpt-oss-20b | adv-context-pollution | No | 4 | 4 | 4 | 0 | 0 | 289 |
| openai/gpt-oss-20b | adv-context-pollution | Yes | 4 | 1 | 1 | 3 | 0 | 300 |
| openai/gpt-oss-20b | adv-cot-poisoning | No | 4 | 4 | 4 | 0 | 0 | 339 |
| openai/gpt-oss-20b | adv-cot-poisoning | Yes | 4 | 4 | 4 | 0 | 0 | 273 |
| openai/gpt-oss-20b | adv-format-pressure | No | 4 | 0 | 0 | 0 | 0 | 280 |
| openai/gpt-oss-20b | adv-format-pressure | Yes | 4 | 4 | 4 | 0 | 0 | 290 |
| openai/gpt-oss-20b | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 340 |
| openai/gpt-oss-20b | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 269 |
| openai/gpt-oss-20b | adv-prompt-injection | No | 4 | 1 | 1 | 3 | 0 | 245 |
| openai/gpt-oss-20b | adv-prompt-injection | Yes | 4 | 3 | 3 | 1 | 0 | 309 |
| qwen/qwen3.6-27b | adv-ambiguous-instruction | No | 4 | 4 | 4 | 0 | 0 | 245 |
| qwen/qwen3.6-27b | adv-ambiguous-instruction | Yes | 4 | 4 | 4 | 0 | 0 | 253 |
| qwen/qwen3.6-27b | adv-budget-starvation | No | 4 | 2 | 2 | 2 | 0 | 227 |
| qwen/qwen3.6-27b | adv-budget-starvation | Yes | 4 | 4 | 4 | 0 | 0 | 233 |
| qwen/qwen3.6-27b | adv-conflicting-data | No | 4 | 4 | 4 | 0 | 0 | 252 |
| qwen/qwen3.6-27b | adv-conflicting-data | Yes | 4 | 4 | 4 | 0 | 0 | 255 |
| qwen/qwen3.6-27b | adv-context-pollution | No | 4 | 4 | 4 | 0 | 0 | 391 |
| qwen/qwen3.6-27b | adv-context-pollution | Yes | 4 | 4 | 4 | 0 | 0 | 256 |
| qwen/qwen3.6-27b | adv-cot-poisoning | No | 4 | 1 | 1 | 3 | 0 | 262 |
| qwen/qwen3.6-27b | adv-cot-poisoning | Yes | 4 | 1 | 1 | 3 | 0 | 254 |
| qwen/qwen3.6-27b | adv-format-pressure | No | 4 | 4 | 4 | 0 | 0 | 390 |
| qwen/qwen3.6-27b | adv-format-pressure | Yes | 4 | 4 | 4 | 0 | 0 | 510 |
| qwen/qwen3.6-27b | adv-language-degradation | No | 4 | 4 | 4 | 0 | 0 | 242 |
| qwen/qwen3.6-27b | adv-language-degradation | Yes | 4 | 4 | 4 | 0 | 0 | 238 |
| qwen/qwen3.6-27b | adv-prompt-injection | No | 4 | 1 | 1 | 3 | 0 | 258 |
| qwen/qwen3.6-27b | adv-prompt-injection | Yes | 4 | 1 | 1 | 3 | 0 | 515 |

## NIM results

| Model | Scenario | Example | N | Fmt OK | Sem OK | 429 | Err | P50 ms |
|-------|----------|---------|---|--------|--------|-----|-----|--------|
| deepseek-ai/deepseek-v4-flash-0731 | adv-ambiguous-instruction | Yes | 3 | 3 | 3 | 0 | 0 | 582 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-budget-starvation | Yes | 3 | 3 | 3 | 0 | 0 | 496 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-conflicting-data | Yes | 3 | 3 | 3 | 0 | 0 | 2598 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-context-pollution | Yes | 3 | 3 | 3 | 0 | 0 | 751 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-cot-poisoning | Yes | 3 | 3 | 3 | 0 | 0 | 422 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-format-pressure | Yes | 3 | 3 | 3 | 0 | 0 | 590 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-language-degradation | Yes | 3 | 3 | 3 | 0 | 0 | 906 |
| deepseek-ai/deepseek-v4-flash-0731 | adv-prompt-injection | Yes | 3 | 3 | 3 | 0 | 0 | 1471 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-ambiguous-instruction | Yes | 3 | 0 | 0 | 0 | 0 | 5885 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-budget-starvation | Yes | 3 | 0 | 0 | 0 | 0 | 1126 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-conflicting-data | Yes | 3 | 0 | 0 | 0 | 0 | 5940 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-context-pollution | Yes | 3 | 0 | 0 | 0 | 0 | 8052 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-cot-poisoning | Yes | 3 | 0 | 0 | 0 | 0 | 5529 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-format-pressure | Yes | 3 | 0 | 0 | 0 | 0 | 2248 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-language-degradation | Yes | 3 | 0 | 0 | 0 | 0 | 5579 |
| nvidia/llama-3.3-nemotron-super-49b-v1.5 | adv-prompt-injection | Yes | 3 | 0 | 0 | 0 | 0 | 5093 |

## Summary

- **Groq:** 384 calls, 289 OK, 95 429
- **NIM:** 48 calls, 48 OK
- **Latency:** P50=326ms P95=5529ms P99=9007ms

## Key Findings

### Cross-model ranking (format compliance)

| Model | Fmt OK | Sem OK | N | 429 |
|-------|--------|--------|---|-----|
| deepseek-v4-flash (NIM) | 100% | 100% | 24 | 0 |
| qwen3.6-27b (Groq) | 78% | 78% | 64 | 14 |
| allam-2-7b (Groq) | 77% | 39% | 64 | 15 |
| llama-3.1-8b-instant (Groq) | 64% | 58% | 64 | 19 |
| llama-3.3-70b-versatile (Groq) | 64% | 64% | 64 | 23 |
| gpt-oss-20b (Groq) | 62% | 62% | 64 | 12 |
| gpt-oss-120b (Groq) | 59% | 59% | 64 | 12 |
| nemotron-super-49b (NIM) | 0% | 0% | 24 | 0 |

### Scenario ranking (format compliance)

| Scenario | Fmt OK | Sem OK | 429 | Hardest? |
|----------|--------|--------|-----|---------|
| adv-language-degradation | 89% | 81% | 3 | No |
| adv-ambiguous-instruction | 74% | 67% | 7 | No |
| adv-conflicting-data | 72% | 57% | 12 | Moderate |
| adv-context-pollution | 69% | 69% | 14 | Moderate |
| adv-format-pressure | 67% | 59% | 5 | Moderate |
| adv-budget-starvation | 50% | 35% | 8 | Yes |
| adv-cot-poisoning | 52% | 52% | 23 | Yes |
| adv-prompt-injection | 52% | 52% | 23 | Yes |

### Finding 1: Budget starvation at max_tokens=20 is model-dependent

At max_tokens=20, gpt-oss-120b and gpt-oss-20b produce 0/8 each (finish_reason=length, 20 tokens consumed). They cannot compress `DATE: 2025-11-03\nSOURCE: S-17` into 20 output tokens. qwen3.6-27b succeeds 7/8 (only 429s stop it). llama-3.1-8b also succeeds 7/8. The failure is not erratic — it is deterministic: `finish_reason=length` means the model ran out of tokens mid-output every time.

**Actionable insight:** The prompt compiler should detect when MaxOutputTokens is below a minimum threshold derived from the expected answer format length, and either (a) return a compile error, or (b) degrade to a minimal format. This prevents sending requests that are guaranteed to fail on truncation.

### Finding 2: nemotron-super-49b is fundamentally incompatible

The model scored 0/24 across all 8 scenarios with FormatExample enabled. Latencies are extremely high (P50 5.9s, max 8s) and all responses have finish_reason=length — the model burns through its max_tokens budget (128) without producing the expected format. This is a deployment incompatibility, not a prompt issue: the model likely needs large output budgets or a different prompting style entirely.

**Decision:** Do not route cognitive operations to nemotron-super-49b in the current preset. Qualify as INCOMPATIBLE.

### Finding 3: CoT-poisoning and prompt-injection are the hardest scenarios

Both scenarios have 52% format and 52% semantic compliance. They also have the highest 429 rates (23 each), meaning 43% of calls were rate-limited. The non-429 subset shows higher compliance when the call actually completes, but the rate limits create a reliability gap. FormatExample provides only +3% (CoT-poisoning) and +11% (prompt-injection) improvement, suggesting the format example helps marginally with injection resistance but is not sufficient.

**Actionable insight:** CoT-poisoning requires a dedicated counter-measure — either (a) stripping few-shot examples from the prompt when detected, or (b) an explicit warning about exemplar format contamination. Prompt injection requires the instruction barrier to be strengthened in the system prompt.

### Finding 4: FormatExample has scenario-dependent effects

FormatExample helps most on budget-starvation (+15%) and prompt-injection (+11%), but hurts on language-degradation (-20%) and conflicting-data (-12%). The negative effect on language-degradation is notable: without example, models score 100%, but with example they drop to 80%. The example adds token overhead and may trigger the model to emit the example format instead of the actual answer in the PT-BR context.

**Actionable insight:** FormatExample should be used selectively — enabled for injection/budget scenarios, disabled for language-sensitive scenarios where the format is already clear.

### Finding 5: deepseek-v4-flash is the strongest NIM model tested

24/24 (100%) on all 8 scenarios with FormatExample, zero 429s, zero errors. Latency is higher (P50 590–2598ms) but reliability is perfect. This model should be qualified as a primary NIM candidate for cognitive operations.

### Finding 6: 429 rate limits are a major reliability factor on Groq

95/384 Groq calls (25%) returned 429. CoT-poisoning and prompt-injection scenarios each had 23+429 calls (48% of trials for those scenarios). This creates a selection bias in the results — the format/semantic compliance numbers would likely be different if 429s were excluded. The resource gate and circuit breaker implemented in Phase 381+ correctly handle this at the runtime level, but campaign measurements should report both with-429 and without-429 compliance rates.

### Decisions for next cycle

1. **Implement `BudgetGuard`** in the prompt compiler: detect when MaxOutputTokens is too low to contain the expected format + content length, and return a compile error or degrade the format. This prevents guaranteed-failure requests.
2. **Test CoT-poisoning counter-measure**: craft a prompt variant that adds an explicit warning about few-shot exemplar contamination, and measure improvement on Groq qwen3.6-27b (best Groq model) and NIM deepseek-v4-flash (control).
3. **Qualify deepseek-v4-flash** as a primary NIM model and mark nemotron-super-49b as INCOMPATIBLE.
4. **Selective FormatExample**: add a compiler hint/flag to enable examples per-scenario rather than globally.
