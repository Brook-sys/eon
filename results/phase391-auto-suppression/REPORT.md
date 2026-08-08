# Phase 391: BudgetGuard Reasoning Effort Auto-Suppression & ModelExecutor Wire Preservation

## Objectives
1. Implement dynamic reasoning effort auto-suppression in `prompt.Compiler` when output token budget (`MaxOutputTokens`) cannot accommodate model reasoning overhead (`ThinkingOverheadTokens`) but exceeds the base answer format floor (`estimateMinOutputTokens`).
2. Expose `ReasoningEffortSuppressed bool` on `prompt.Result` for audit tracing.
3. Update `kernel.ModelExecutor` across all 6 request construction sites to preserve `compiled.Request.ReasoningEffort` when `plan.ReasoningEffort` is empty (preventing silent wiping of auto-suppressed `"none"`).
4. Conduct a live fire campaign on Groq `qwen/qwen3.6-27b` under tight output token budgets (`max_tokens=64`) to empirically validate auto-suppression vs. unsuppressed budget starvation.

## Implementation Details
- `internal/prompt/compiler.go`:
  - Extended `prompt.Result` with `ReasoningEffortSuppressed bool`.
  - In `Compiler.Compile`: evaluated `minOutputBase := estimateMinOutputTokens(input.AnswerFormat)`.
  - When `spec.MaxOutputTokens < minOutput` (where `minOutput = minOutputBase + input.ThinkingOverheadTokens`) AND `input.ThinkingOverheadTokens > 0`:
    - If `spec.MaxOutputTokens >= minOutputBase`, auto-suppress: set `minOutput = minOutputBase`, `reasoningSuppressed = true`, `Request.ReasoningEffort = "none"`.
    - Else (when `spec.MaxOutputTokens < minOutputBase`), fail closed with `ErrOutputBudgetInsufficient`.
- `internal/kernel/model_executor.go`:
  - Updated all 6 `request.ReasoningEffort` assignment sites to `if plan.ReasoningEffort != "" { request.ReasoningEffort = plan.ReasoningEffort }`. This preserves compiler-injected `"none"` when the provider profile has an empty default.
- `internal/prompt/compiler_test.go`:
  - Added unit test `TestBudgetGuardAutoSuppressesReasoningEffort` verifying auto-suppression behavior and `ReasoningEffortSuppressed` flag.
  - Adjusted `TestBudgetGuardIncludesThinkingOverhead` to verify failure when budget is below both format floor and thinking overhead.

## Live Fire Campaign Results (Groq `qwen/qwen3.6-27b`, 2026-08-08)

| Case | max_tokens | Thinking Overhead | Suppressed? | Wire Effort | Latency | Out Tokens | Finish Reason | Parsed Output |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| auto-suppress-tight-budget | 64 | 384 | **true** | `"none"` | 331ms | 9 | **stop** | STATUS: "OK", CONFIRM: "YES" ✓ |
| unsuppressed-tight-budget-control | 64 | 0 | **false** | `""` | 329ms | 64 | **length** | (empty, budget starved) ✗ |
| auto-suppress-moderate-budget | 128 | 384 | **true** | `"none"` | 360ms | 9 | **stop** | STATUS: "OK", CONFIRM: "YES" ✓ |

### Key Empirical Discoveries

1. **Auto-Suppression Prevents Budget Starvation**:
   Without auto-suppression (control case), `qwen/qwen3.6-27b` under `max_tokens=64` spends all 64 output tokens on shadow thinking (<think>...</think>), hitting `finish_reason=length` without emitting the structured answer. With auto-suppression, the model emits `STATUS: OK\nCONFIRM: YES` in **9 tokens** (~331ms) with `finish_reason=stop`.

2. **Zero Overhead for Structured Answers**:
   Suppressing reasoning effort when output budgets are tight yields a **7x reduction in output tokens** (9 vs 64+ tokens) and avoids output truncation errors.

3. **Wire Preservation Verified**:
   `ModelExecutor` correctly preserves `Request.ReasoningEffort = "none"` down to provider execution when `plan.ReasoningEffort` is empty, ensuring end-to-end integration between compiler auto-suppression and transport payload generation.

## Verification
- `go test ./...` 100% passing across all packages (no failures).
- `git diff --check` clean.
- Live fire campaign executed on Groq `qwen/qwen3.6-27b` with exact-match verification.
- Output artifacts written to `results/phase391-auto-suppression/results.json`.
