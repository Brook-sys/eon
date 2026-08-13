# Phase 486 - Groq Model Discovery & Qualification

**Objective:** Discover currently available models on Groq via `/v1/models` and qualify any new/untested models that fit the text-to-text contract, ensuring our evaluation campaigns rotate across the full available deployment.

**Observation:**
Running `GET https://api.groq.com/openai/v1/models` via standard OpenAI-compatible client returned 15 models.
Relevant LLM models discovered:
- `llama-3.3-70b-versatile` (already actively tested)
- `llama-3.1-8b-instant` (already actively tested)
- `qwen/qwen3.6-27b` (already actively tested)
- `openai/gpt-oss-120b` (already actively tested)
- `openai/gpt-oss-20b` (already actively tested)
- `openai/gpt-oss-safeguard-20b` (New to our test scope, specifically aimed at safety/guardrails)
- `groq/compound` and `groq/compound-mini` (New to our test scope, undocumented internal ensemble/mixture models)
- `allam-2-7b` (New to our test scope, Arabic-focused model)
- `canopylabs/orpheus-v1-english` / `canopylabs/orpheus-arabic-saudi` (New, canopy labs)

**Action Plan:**
We will create a small campaign to baseline the untested models (`openai/gpt-oss-safeguard-20b`, `groq/compound`, `groq/compound-mini`, `allam-2-7b`, `canopylabs/orpheus-v1-english`) against our adversarial formats and context windows.

This ensures we aren't repeatedly grinding the same 5 models and expands our resiliency profiles.

**Test Campaign Results:**
1. `openai/gpt-oss-safeguard-20b` returned an empty response. Being a safeguard model, it's likely expecting different inputs (e.g. evaluating safe/unsafe content) and rejecting standard generative completion prompts.
2. `groq/compound` completely failed the structural instruction test, engaging in internal CoT monologue ("I considered the user's instructions carefully...") instead of emitting `SYSTEM_ONLINE` only. This model appears unsuitable for strict text-to-text exact-match pipelines and overrides basic system instructions.
3. `groq/compound-mini` successfully responded with `SYSTEM_ONLINE`. However, on the first pass it threw a 429 rate limit because it seems to internally route to `openai/gpt-oss-120b` (the error specifically cited: `Rate limit reached for model openai/gpt-oss-120b in organization ... type:compound`).
4. `allam-2-7b` successfully responded with `SYSTEM_ONLINE ` (with a trailing space).
5. `canopylabs/orpheus-v1-english` requires external terms acceptance on the Groq Console, throwing HTTP 400 `model_terms_required`.

**Conclusion for Phase 486:**
- `allam-2-7b` can be added to the available experimental catalog.
- `groq/compound` is highly resistant to constraint binding and fails basic compliance tests.
- `groq/compound-mini` routes to other models and will randomly fail based on *their* rate limits (like gpt-oss-120b), making it unpredictable for robust architecture constraints.
- `openai/gpt-oss-safeguard-20b` is not a generative instruct model in the same sense.
- `canopylabs` models are currently blocked by terms of service.

The text-to-text contract is best maintained by explicit model specifications (Llama 3.1/3.3, Qwen, GPT-OSS). Compound models break determinism in our pipeline.
