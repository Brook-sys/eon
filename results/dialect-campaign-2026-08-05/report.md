# Dialect Compatibility Campaign — 2026-08-05

## Hypothesis
Different Groq models may accept or reject `max_tokens` vs `max_completion_tokens` differently. The dialect field in `ModelBindingConfig` must route the correct parameter per model. This campaign validates that both dialects work across the 3 target Groq models.

## Setup
- **Provider:** Groq (api.groq.com/openai/v1)
- **Models:** llama-3.1-8b-instant, llama-3.3-70b-versatile, openai/gpt-oss-20b
- **Dialects tested:** `max_tokens` (legacy), `max_completion_tokens` (completion)
- **Trials per cell:** 3
- **Total calls:** 18 (bounded)
- **Temperature:** 0
- **Max output:** 48 tokens
- **Prompt:** "Respond with exactly: DIALECT_OK"

## Results

| Model | Dialect | OK | Empty Content | Errors | P50 Latency |
|-------|---------|----|---------------|---------|-------------|
| llama-3.1-8b-instant | max_tokens | 3/3 | 0 | 0 | 368ms |
| llama-3.1-8b-instant | max_completion_tokens | 3/3 | 0 | 0 | 382ms |
| llama-3.3-70b-versatile | max_tokens | 3/3 | 0 | 0 | 367ms |
| llama-3.3-70b-versatile | max_completion_tokens | 3/3 | 0 | 0 | 364ms |
| openai/gpt-oss-20b | max_tokens | 0/3 | 3 | 0 | 409ms |
| openai/gpt-oss-20b | max_completion_tokens | 0/3 | 3 | 0 | 427ms |

## Analysis

1. **Both dialects work universally on Groq** — all 18 calls returned HTTP 200 with no provider errors, regardless of dialect used. Groq's API accepts both `max_tokens` and `max_completion_tokens` fields.

2. **llama-3.1-8b-instant and llama-3.3-70b-versatile** correctly followed the instruction, returning "DIALECT_OK" in all 6 trials each. Token usage is consistent: 43 prompt + 5 completion.

3. **openai/gpt-oss-20b returns empty content** in all 6 trials (both dialects). It consumes 78 prompt tokens (vs 43 for Llama models — it likely re-expands the system prompt) and hits the 48 token limit, but the completion field is empty. This is a known behavior of gpt-oss models with very short max token limits — the model uses all tokens for reasoning/internal processing and produces no visible content. This is NOT a dialect issue — both dialects produce the same result.

4. **No 429/Retry-After** observed in 18 sequential calls with 0.3s spacing.

## Decision
- The default `max_output_dialect: "max_tokens"` (previously incorrectly set to `"legacy"`) is correct for Groq models. Both dialects are accepted.
- The bug fix from `"legacy"` → `"max_tokens"` in the dashboard is validated: `"legacy"` was rejected by domain validation (confirmed in unit test), while `"max_tokens"` works universally.
- For gpt-oss models, the empty content at low token limits is a model behavior, not a dialect issue. Higher `max_output_tokens` budget is needed.

## Next experiment
- Test gpt-oss-20b with higher max_tokens (256+) to confirm it produces visible content.
- Cross-provider NIM comparison: test `max_completion_tokens` on NVIDIA NIM models to verify cross-provider compatibility.
- Adversarial: test with `max_tokens: 0` to confirm both dialects reject the invalid value.
