# Phase 484 — Adversarial Retry Campaign for Qwen3.6-27B (Reasoning Impact)

**Objective**: Verify the effect of the `reasoning_effort` flag (`none` vs `low`) and auto-recovery mechanics on Groq's `qwen/qwen3.6-27b` under adversarial prompts (ambiguous context, polluted context, budget starvation, and CoT poisoning).

**Live Evidence**:
The campaign executed 10 trials against `qwen/qwen3.6-27b` varying `reasoning_effort`.

- **With `reasoning_effort: "none"` (and `reasoning_format: "parsed"`)**:
  - `adv-ambiguous`: 100% compliance (`CONFLICT: YES`, 5 tokens).
  - `adv-polluted_context`: 100% compliance (`CONFLICT: NO`, 5 tokens).
  - `adv-budget_starvation`: Auto-recovery worked (Retry with 50 tokens succeeded cleanly returning `CONFLICT: YES`).
  - `adv-cot_poisoning`: 100% compliance (`CONFLICT: NO`, 5 tokens).
  - **Verdict**: Complete success. `reasoning_effort: none` ensures Qwen returns bounded strings, cleanly passing format requirements without wasting tokens in `<think>`.

- **With `reasoning_effort: "low"` (no reasoning_format override)**:
  - `adv-ambiguous`: `finish_reason=length`. Output consisted entirely of `<think>Here's a thinking process...` (200 tokens exhausted).
  - `adv-polluted_context`: `finish_reason=length`. Tokens exhausted in `<think>`.
  - `adv-budget_starvation`: `finish_reason=length`. Auto-recovery attempt failed because even the retried 50 tokens were completely consumed by the `<think>` preamble.
  - `adv-cot_poisoning`: `finish_reason=length`. Tokens exhausted in `<think>`.
  - **Verdict**: Total failure for exact-match short-output tasks. The model's shadow reasoning consumes all budget before emitting the required output.

**Conclusion & Actionable Insight**:
For classification and bounded parsing tasks, it is mandatory to enforce `reasoning_effort: "none"` (with format `"parsed"`) for `qwen/qwen3.6-27b` on Groq. Without this explicit override, Qwen defaults to deep shadow reasoning, immediately violating bounded contracts (budget starvation) and breaking structured parsers. The prompt auto-recovery from Phase 467 works well when reasoning is suppressed, but cannot save a budget-exhausted thinking trace.
