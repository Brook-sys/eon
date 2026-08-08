# Phase 390: Reasoning Effort Pipeline Integration & Probe Resiliency

## Objectives
1. Wire `DefaultReasoningEffort` from `ProviderProfile` through `AdaptationPlan` to `port.CompletionRequest` in kernel `ModelExecutor`.
2. Refactor `prompt.ParseResponse` to leverage `modeltext.NormalizeStructuredResponse` (stripping BOM, thinking tags, and markdown code fences in a unified ladder).
3. Enhance `provider/openai.Provider.Probe` to automatically retry without `ReasoningEffort` when standard baseline models (e.g. Groq `llama-3.3-70b-versatile`) reject the `reasoning_effort` wire parameter with HTTP 400.
4. Execute a live fire campaign on Groq + NIM verifying probe confirmation, reasoning effort passthrough, and structured response parsing across multiple active model families.

## Implementation Details
- `internal/domain/provider_profile.go`: Added `DefaultReasoningEffort string` field to `ProviderProfile` with validation for `none`, `low`, `medium`, `high`.
- `internal/domain/model_adaptation.go`: Extended `AdaptationPlan` with `ReasoningEffort` and updated `SelectAdaptationPlan` to propagate `DefaultReasoningEffort` from profile.
- `internal/kernel/model_executor.go`: Wired `request.ReasoningEffort = plan.ReasoningEffort` across initial execution and fallback/retry branches (6 sites).
- `internal/prompt/response_parser.go`: Refactored `ParseResponse` to run `modeltext.NormalizeStructuredResponse` as a pre-parse ladder step, removing inline thinking/fence stripping logic.
- `internal/provider/openai/provider.go`: Added error recovery in `Probe` to retry without `ReasoningEffort` on HTTP 400.

## Live Fire Campaign Results (8 trials, 2026-08-08)

| Case | Model | Probe | Latency | In Tk | Out Tk | Thought Stripped | Parsed |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| weekday-baseline | llama-3.3-70b-versatile | ✓ (auto fallback) | 321ms | 71 | 25 | no | DATE: 2026-08-08, WEEKEND: YES ✓ |
| weekday-effort-none | qwen/qwen3.6-27b | ✓ (effort=none) | 227ms | 57 | 20 | no | DATE: 2026-08-08, WEEKEND: YES ✓ |
| weekday-baseline-8b | llama-3.1-8b-instant | ✓ (auto fallback) | 224ms | 71 | 16 | no | DATE: 2026-08-08, WEEKEND: NO ✗ |
| conflict-detection-70b | llama-3.3-70b-versatile | ✓ | 344ms | 79 | 26 | no | CONFLICT: YES, REASON: discrepancy ✓ |
| conflict-detection-8b | llama-3.1-8b-instant | ✓ | 287ms | 79 | 33 | no | CONFLICT: YES, REASON: ✓ (semantically reasonable) |
| conflict-detection-qwen | qwen/qwen3.6-27b | ✓ | 259ms | 62 | 37 | no | CONFLICT: YES, REASON: discrepancy ✓ |
| thinking-tags-resilience | qwen/qwen3.6-27b | ✓ | 690ms | 59 | 256 | **yes** | **Budget starvation** — thinking filled 256 token budget, no structured output ✗ |
| nim-control-8b | meta/llama-3.1-8b-instruct | ✓ | 2135ms | 71 | 16 | no | DATE: 2026-08-08, WEEKEND: NO ✗ |

### Key Findings

**F1: 8B models fail weekday computation (cognitive gap, not format)**
Both Groq `llama-3.1-8b-instant` and NIM `meta/llama-3.1-8b-instruct` answer `WEEKEND: NO` for 2026-08-08 (a Saturday — verified independently). Format is correct, parsing succeeds, but the semantic answer is wrong. The 70B and 27B models answer correctly. This is a model capacity limitation, not a pipeline bug.

**F2: Budget starvation when thinking is not disabled**
`qwen/qwen3.6-27b` without `reasoning_effort=none` on the thinking-tags-resilience case produces 256 output tokens of thinking content (StripThinkingTags: true), exhausting the output budget before emitting the structured KEY: value answer. With `reasoning_effort=none` (weekday-effort-none case), the same model produces a clean 20-token structured response. This validates the importance of the `DefaultReasoningEffort` pipeline: reasoning-capable models need explicit effort suppression for structured output tasks with bounded budgets.

**F3: Probe auto-fallback works correctly**
`llama-3.3-70b-versatile` and `llama-3.1-8b-instant` both initially send `reasoning_effort=none` in the probe, receive HTTP 400, and automatically retry without it. Both probes succeed. This confirms the probe resilience fix works on live endpoints.

**F4: Cross-provider 8B consistency**
NIM `meta/llama-3.1-8b-instruct` produces the same wrong answer as Groq `llama-3.1-8b-instant` for weekday detection — the cognitive limitation is in the model family, not the deployment.

**F5: Conflict detection is robust across model sizes**
All three Groq models (70B, 27B, 8B) correctly identify the conflict and produce reasonable explanations. The 8B's explanation ("Sensor B reports a latency that is greater than the window's duration, which is likely not possible") is semantically different but still valid reasoning.

## Verification
- `go test ./...` 100% passing (all packages).
- `git diff --check` clean.
- 8 live calls to real providers (7 Groq + 1 NIM), 0 errors, 0 429s.
- Probe auto-fallback verified on live endpoints.
- Budget starvation finding recorded for future prompt compiler improvements.

## Decisions
- The `ReasoningEffort` pipeline is verified and ready for main.
- Budget starvation finding (F2) motivates a future improvement: prompt compiler should reserve output budget for structured answer when `reasoning_effort` is unset on reasoning-capable models.
- 8B cognitive gap (F1) is a known model limitation, not actionable at the pipeline level.
