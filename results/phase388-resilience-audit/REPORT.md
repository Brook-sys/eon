# Phase 388 Resilience Audit: Prompt starvation & reasoning budget controls

**Objective:** Audit behavior of multiple Groq models under deep output token starvation (max_tokens=10 to 20) with and without explicit reasoning_effort controls, following Phase 384 feature parity. Ensure baseline models comply and accurately map adapter errors for hybrid reasoning models.

**Methodology:**
- Executed isolated test harnesses over `openai.New(openai.Config{...})`
- Target baseline exact response: `"READY"`
- Parameters: `Temperature=0.0`, `MaxOutputTokens=10` (later 20), varying `ReasoningEffort`.
- Tested Models: `llama-3.1-8b-instant`, `llama-3.3-70b-versatile`, `allam-2-7b`, `openai/gpt-oss-20b`, `qwen/qwen3.6-27b`. (And tested NIM deepseek/mixtral/gemma out of band - all rejected due to unknown model ID on Groq).

**Key Findings:**
1. **Llama 3 family (`llama-3.1-8b-instant`, `llama-3.3-70b-versatile`)**: Highly resilient. Perfect 100% compliance returning exactly `"READY"` within 10 tokens (uses 2 tokens).
2. **Qwen 3.6 (`qwen/qwen3.6-27b`)**:
    - With `reasoning_effort=none`: 100% compliant, returns `"READY"`, consumes 2 output tokens.
    - Without `reasoning_effort`: FAILS. Consumes tokens into `<think>` tags and hits length limits before emitting semantic content.
3. **OpenAI GPT-OSS (`openai/gpt-oss-20b`)**: 
    - Fails under extreme budget starvation (`max_tokens <= 20`), even with `reasoning_effort="low"`. The adapter accurately detects this and returns `INVALID_RESPONSE: reasoning_budget_exhausted` since internal reasoning tokens consume the budget and `finish_reason=length` is returned without content.
4. **Allam (`allam-2-7b`)**:
    - Fails exact string matching due to trailing spaces `"READY "`. Consumes 4 tokens.

**Decisions:**
- The `reasoning_effort="none"` passthrough in the OpenAI adapter (Phase 384) remains the primary requirement to unblock `qwen3.6-27b` on extraction tasks.
- `openai/gpt-oss-*` models require larger minimum output budgets (recommended >= 64 tokens) to account for baseline reasoning overhead, even when `reasoning_effort="low"`.
- `allam-2-7b` continues to exhibit minor format discrepancies (trailing space) alongside its reasoning deficits. Normalizer loops must trim outputs securely.

**Artifacts:**
- `results/phase388-resilience-audit/results.json` (Sanitized)
- `scripts/phase388_run.go` (Audit script)
