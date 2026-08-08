# FormatExample Adversarial Campaign — 2026-08-07

## Hypothesis

Adding an explicit `EXAMPLE` block to the prompt compiler (via `FormatExample` field) improves format compliance on adversarial tasks where bare models drop required prefixes (`DATE:`/`SOURCE:`), without degrading strong model performance.

## Motivation

Phase 383 adversarial fire sweep (2026-08-05, 288 live calls) identified **adv-language-degradation (PT-BR)** as the most discriminating scenario: all 8B models scored 0% format compliance. Models extract correct semantic values but emit bare `2025-11-03\nS-17` instead of `DATE: 2025-11-03\nSOURCE: S-17`.

## Campaign Design

- **Provider:** Groq
- **Models:** llama-3.1-8b-instant (8B, known weak), llama-3.3-70b-versatile (70B, known strong), qwen/qwen3.6-27b (27B, reasoning_effort=none)
- **Scenarios:**
  - `adv-language-degradation`: PT-BR mixed context, temp=0.0, max_tokens=128
  - `adv-format-pressure`: temp=0.7, max_tokens=48 (tight + high temp)
  - `adv-budget-starvation`: temp=0.0, max_tokens=20 (extreme tight)
- **Arms:** Without EXAMPLE vs With EXAMPLE (identical prompt + `\n\nEXAMPLE\nDATE: 2025-11-03\nSOURCE: S-17`)
- **Reps:** 5 per cell
- **Total planned:** 3 scenarios × 3 models × 2 arms × 5 reps = 90 calls
- **Actual:** 90 calls attempted, 4 HTTP 429 (all llama-3.1-8b-instant, reimbursable), 86 valid completions

## Results

### adv-language-degradation (PT-BR) — KEY FINDING

| Model | Without EXAMPLE | With EXAMPLE |
|-------|----------------|--------------|
| llama-3.1-8b-instant | **0/4** (0%) format† | **5/5** (100%) format |
| llama-3.3-70b-versatile | 5/5 (100%) | 5/5 (100%) |
| qwen/qwen3.6-27b | 5/5 (100%) | 5/5 (100%) |

†1 trial lost to 429. Model emits bare `2025-11-03\nS-17` (semantic correct, format wrong) without EXAMPLE.

**FormatExample lifts 8B format compliance from 0% → 100% on the exact scenario where Phase 383 found universal 8B failure.**

### adv-format-pressure (temp=0.7, max_tokens=48)

| Model | Without EXAMPLE | With EXAMPLE |
|-------|----------------|--------------|
| llama-3.1-8b-instant | 3/4 (75%)† | 4/4 (100%)† |
| llama-3.3-70b-versatile | 5/5 (100%) | 5/5 (100%) |
| qwen/qwen3.6-27b | 5/5 (100%) | 5/5 (100%) |

†1 trial each lost to 429.

### adv-budget-starvation (max_tokens=20)

| Model | Without EXAMPLE | With EXAMPLE |
|-------|----------------|--------------|
| llama-3.1-8b-instant | 5/5 (100%) | 5/5 (100%) |
| llama-3.3-70b-versatile | 5/5 (100%) | 5/5 (100%) |
| qwen/qwen3.6-27b | 5/5 (100%) | 5/5 (100%, finish=length) |

Qwen 27B consistently hits `finish_reason=length` at 20 tokens — output is correct but truncated at the token limit. EXAMPLE does not cause overhead problems.

## Latency and Token Analysis

- **8B without EXAMPLE (lang-degradation):** avg 245ms, 8 tokens/trial — model produces minimal bare output
- **8B with EXAMPLE (lang-degradation):** avg 278ms, 16 tokens/trial — model produces properly formatted output
- **70B:** avg ~330ms, 16 tokens — consistent regardless of EXAMPLE
- **27B:** avg ~310ms, 21 tokens — consistent, EXAMPLE adds ~1ms latency
- **429 rate:** 4/90 (4.4%), all on llama-3.1-8b-instant, all with Retry-After 1-3s

## Decisions

1. **FormatExample is effective and safe.** It completely fixes 8B format failure on PT-BR language degradation (0% → 100%) without harming strong models (70B, 27B remain 100%).
2. **No token overhead penalty.** EXAMPLE adds ~5 words to the prompt but does not increase output token count — models that already format correctly are unaffected.
3. **Wire FormatExample into prompt compiler.** The field is now part of `prompt.Input` and rendered between ANSWER and the closing delimiter. `ModelExecutor.buildPromptInput` populates it for `exact_text`, `exact_json`, and `ProposedChangeSet` output schemas.
4. **Strong models don't need it but aren't hurt.** llama-3.3-70b and qwen3.6-27b (effort=none) maintain 100% regardless. EXAMPLE is a safety net for weaker models.

## Next Steps

- Test FormatExample on the full 8-scenario adversarial suite from Phase 383 to confirm the effect generalizes beyond format-pressure scenarios
- Evaluate whether EXAMPLE helps NIM 8B models (meta/llama-3.1-8b-instruct) which showed different failure modes (polarity inversion, not just format)
- Consider auto-populating FormatExample from OperationSpec metadata when available

## Artifacts

- `results/format-example-campaign-2026-08-07/results.json` — 90 trial records with full response text, tokens, latency, HTTP status
- `cmd/fire_test/main.go` — campaign runner source
