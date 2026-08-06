# Adversarial Fire Sweep — 2026-08-05

## Scope

Two bounded live-fire campaigns across Groq (6 models) and NVIDIA NIM (4 models),
covering all 8 adversarial scenarios from the operator's directive (2026-08-01).

### Campaign A: Groq adversarial sweep
- **Slice:** `adv-fire-2026-08-05`
- **Models:** llama-3.3-70b-versatile, llama-3.1-8b-instant, openai/gpt-oss-20b, openai/gpt-oss-120b, qwen/qwen3.6-27b, allam-2-7b
- **Tasks:** 8 adversarial scenarios × 5 reps × temp=0.0
- **Calls planned:** 240
- **Calls ok:** 202 | **Errors (429):** 38
- **Latency:** P50=423ms P95=663ms Max=2318ms

### Campaign B: NIM cross-provider adversarial sweep
- **Slice:** `adv-fire-nim-2026-08-05`
- **Models:** meta/llama-3.1-8b-instruct, meta/llama-3.1-70b-instruct, nvidia/llama-3.3-nemotron-super-49b-v1, mistralai/mistral-nemotron
- **Tasks:** 4 hardest adversarial scenarios × 3 reps × temp=0.0
- **Calls planned:** 48
- **Calls ok:** 46 | **Errors (transport):** 2
- **Latency:** P50=1026ms P95=22756ms Max=28489ms

## Key findings

### 1. Format compliance is the dominant failure mode

Most "incorrect" responses extract the **correct semantic data** but fail to emit
the required `KEY: value` delimited format. The models know the answer; they just
don't follow the format constraint under adversarial conditions.

**Pattern:** Models drop the `DATE:` / `SOURCE:` prefix and emit bare values
like `2025-11-03\nS-17` instead of `DATE: 2025-11-03\nSOURCE: S-17`.

### 2. adv-language-degradation (PT-BR) is the hardest scenario

| Model | Provider | Accuracy |
|-------|----------|----------|
| qwen/qwen3.6-27b | Groq | 5/5 (100%) ✅ |
| openai/gpt-oss-120b | Groq | 3/3 (100%) ✅ |
| mistralai/mistral-nemotron | NIM | 3/3 (100%) ✅ |
| meta/llama-3.1-70b-instruct | NIM | 2/2 (100%) ✅ |
| llama-3.3-70b-versatile | Groq | 0/5 (0%) ❌ |
| openai/gpt-oss-20b | Groq | 0/4 (0%) ❌ |
| llama-3.1-8b-instant | Groq | 0/2 (0%) ❌ |
| allam-2-7b | Groq | 0/3 (0%) ❌ |
| nvidia/nemotron-super-49b | NIM | 0/3 (0%) ❌ |
| meta/llama-3.1-8b-instruct | NIM | 0/3 (0%) ❌ |

**Insight:** Large models (120B, 70B NIM, Mistral-Nemotron) and qwen3.6-27b
(with reasoning_effort=none) maintain format compliance under PT-BR mixed language.
Small models (8B) and mid-size models without reasoning suppression fail.

### 3. adv-budget-starvation reveals model-specific token floors

| Model | Provider | Accuracy | Pattern |
|-------|----------|----------|---------|
| qwen/qwen3.6-27b | Groq | 4/4 (100%) | reasoning_effort=none frees budget |
| openai/gpt-oss-120b | Groq | 4/4 (100%) | reasoning_effort=low sufficient |
| llama-3.3-70b-versatile | Groq | 4/4 (100%) | no reasoning overhead |
| mistralai/mistral-nemotron | NIM | 3/3 (100%) | clean formatting |
| nvidia/nemotron-super-49b | NIM | 3/3 (100%) | clean formatting |
| meta/llama-3.1-70b-instruct | NIM | 2/2 (100%) | correct |
| openai/gpt-oss-20b | Groq | 0/4 (0%) | drops SOURCE: prefix |
| llama-3.1-8b-instant | Groq | 0/4 (0%) | bare values |
| allam-2-7b | Groq | 0/3 (0%) | garbled output |
| meta/llama-3.1-8b-instruct | NIM | 0/3 (0%) | bare values |

### 4. adv-conflicting-data — allam-2-7b semantic failure

allam-2-7b returns `NO` (no conflict) where the correct answer is `YES` —
a genuine semantic failure, not a format issue. 3/3 reps consistently wrong.
This model lacks reasoning capability for the conflict-detection task.

### 5. Throttling characterization (Groq)

- 38/240 calls (15.8%) hit HTTP 429 with `Retry-After: 1-3s`
- All 429s recovered on retry or were already the 2nd retry attempt
- Throttle distribution: concentrated on later tasks in sequence per model
- No sustained 429 storms; pattern is consistent with per-minute rate limits
- Retry-After values: 1s (15%), 2s (78%), 3s (7%)

### 6. NIM latency characteristics

- P50 latency: 1026ms (3x slower than Groq P50 of 423ms)
- P95 latency: 22756ms (driven by llama-3.1-70b outliers at 25-28s)
- mistral-nemotron prompt-injection: P50=16535ms (very slow)
- 2 transport timeouts on llama-3.1-70b (45s timeout hit)

## Implications for engine design

1. **Response parser must be format-tolerant**: When `DATE: ...` prefix is missing,
   attempt fallback parsing of bare values by position. The semantic data is often
   correct even when the format is wrong.

2. **Model selection per task class**: 
   - Format-heavy tasks → prefer llama-3.3-70b, qwen3.6-27b (reasoning_effort=none), gpt-oss-120b
   - Avoid 8B models for format-compliance tasks unless prompt reinforcement is added
   - allam-2-7b should not be used for conflict-detection tasks

3. **PT-BR tasks need stronger format anchoring**: The `adv-language-degradation`
   scenario is the most discriminating. Models that maintain format in PT-BR
   are reliable across all other adversarial scenarios.

4. **Throttling is bounded and recoverable**: Concurrency=3 with 250ms inter-call
   delay produces ~16% 429 rate on Groq but all recover within 1-3s. No need for
   exponential backoff; linear retry-after respect is sufficient.

5. **reasoning_effort=none for qwen3.6-27b is critical**: Without it, the model
   consumes all output tokens in shadow reasoning. With `effort=none`, it achieves
   100% accuracy across all 8 adversarial scenarios — the best of any model tested.
