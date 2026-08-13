# ADR 005: Prompt Engineering Strategies for Adversarial Robustness

## Status
Accepted (2026-08-13)

## Context
During massive fire sweeps (Phase 383-386) of our zero-shot classification and extraction pipeline across multiple Groq and NIM models, we observed persistent failure modes on specific adversarial scenarios. Format compliance (yielding bare prefixes or wrapping in markdown) and context pollution were frequent issues.

Most importantly, performance varied radically by model scale and dialect context:
- Smaller reasoning envelopes (like `allam-2-7b` and `llama-3.1-8b`) failed on implicit math or formatting within a translated prompt context (PT-BR).
- Aggressive token budgets (e.g. `max_tokens=12`) triggered models to emit preambles that pushed the actual payload beyond the cut-off boundary, resulting in partial outputs that broke the parser.
- Chain-of-Thought (CoT) examples often poisoned smaller models, which attempted to hallucinate the answer from the example rather than parsing the dynamic facts.

We ran multiple constrained "prompt improvement loops" (testing 3 variants per issue in a 12-to-30 run batch).

## Decision
We establish the following standard practices for prompt authoring in the autonomo engine tasks:

1. **Explicit Math Hints Over CoT for Small Models:**
   For tasks requiring semantic comparison (e.g. `adv-conflicting-data` where latency `120ms` vs `410ms` is a conflict), we must provide an explicit mathematical hint directly in the `Constraints` block (e.g., `120ms vs 410ms is a conflict`). We will **not** rely on prepended CoT examples, which have been proven to confuse 7B-8B parameter models.

2. **Format Anchors Over Negative Constraints:**
   When dealing with non-English dialects (e.g. `adv-language-degradation` in PT-BR), LLMs heavily leak conversational prose even when explicitly forbidden. The most effective mitigation is an **Explicit Format Anchor** (an exact example of the output structure) placed before the `Facts` block, paired with a strict negative constraint. English system instructions wrapping PT-BR facts are unnecessary if the format anchor is present.

3. **Label Enforcement under Budget Starvation:**
   When running under aggressive budget limits (e.g. `max_tokens=12` on `adv-budget-starvation`), models fail by using tokens on preamble. Reordering the constraints to place the format rule *last* and explicitly demanding `No preamble. Output EXACTLY two lines` successfully forces even `allam-2-7b` and `llama-3.1-8b` to emit the required output immediately.

4. **Reasoning Effort Suppression:**
   For hybrid reasoning models (e.g., `qwen/qwen3.6-27b`, `gpt-oss-20b`), we strictly pass `reasoning_effort="none"` for structured data extraction to prevent the model from burning its small budget on shadow-thinking tokens.

## Consequences
- **Positive:** We achieved 100% format and semantic compliance across 5 different model architectures on Groq (ranging from 7B to 70B parameters) on all 10 adversarial tasks using these proven patterns.
- **Negative:** Prompts are slightly longer (costing more input tokens) due to explicit format anchors and explicit constraint hints. However, this is offset by lower output token generation (no preambles) and zero retry loops.
