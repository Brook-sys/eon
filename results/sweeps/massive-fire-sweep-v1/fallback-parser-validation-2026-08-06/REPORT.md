# Fallback Parser Validation Campaign — 2026-08-06

## Hypothesis

The 2026-08-05 adversarial fire sweep found that models (especially 8B)
drop `DATE:`/`SOURCE:` prefixes and emit bare values. The fallback
positional parser in `runner.py` should recover semantically correct
responses that lack format prefixes, while the new `adv-ptbr-format-anchor`
task (with an explicit format example) should prevent format dropping
at the prompt level.

## Scenario

- **Models (4):** Groq llama-3.1-8b-instant, Groq llama-3.3-70b-versatile,
  Groq qwen/qwen3.6-27b (reasoning_effort=none), NIM meta/llama-3.1-8b-instruct
- **Tasks (2):** adv-language-degradation (most discriminating from prior sweep),
  adv-ptbr-format-anchor (new task with format example)
- **Reps:** 5 per model×task cell
- **Temperature:** 0.0
- **Max tokens:** 256
- **Concurrency:** 3, inter-call delay 250ms
- **Total calls:** 40 (all completed, 0 errors, 0 throttles)

## Results

| Model | adv-language-degradation | adv-ptbr-format-anchor | Aggregate |
|---|---|---|---|
| groq/llama-3.3-70b-versatile | 0/5 | 5/5 | 5/10 (50%) |
| groq/llama-3.1-8b-instant | 5/5 | 5/5 | 10/10 (100%) |
| groq/qwen/qwen3.6-27b | 5/5 | 5/5 | 10/10 (100%) |
| nvidia_nim/meta/llama-3.1-8b-instruct | 5/5 | 5/5 | 10/10 (100%) |
| **Total** | **15/20 (75%)** | **20/20 (100%)** | **35/40 (87.5%)** |

## Key Findings

### F1: Format anchor example is the single most effective prompt intervention

The `adv-ptbr-format-anchor` task adds:
1. An explicit example of the expected response format
2. A final instruction: "O formato deve ser exatamente DATE: seguido do
   valor, depois SOURCE: seguido do valor, cada um em sua própria linha."

This achieves **100% format compliance** across all 4 models including
the 70B that fails the non-anchored version 0/5. The 70B produces fluent
prose ("A data de publicação original da fonte S-17 é 2025-11-03.")
instead of `DATE: 2025-11-03` when no example is given, but conforms
perfectly when an example is present.

**Implication:** The prompt compiler should include a format example
for DELIMITED tasks when the target model is known to produce prose
(specifically 70B-class models under PT-BR language pressure).

### F2: Fallback parser did NOT trigger in this campaign

All 30 correct responses used proper `DATE:`/`SOURCE:` prefix format.
The fallback positional parser (bare-value recovery) was not exercised
by live data in this run. It remains as a safety net for the cases
observed in the 2026-08-05 sweep where some models did emit bare values.

The fallback parser is still valuable: the 2026-08-05 sweep covered
6 Groq + 4 NIM models across 8 tasks with 5 reps each. This campaign
only covers 4 models on 2 tasks. The fallback will trigger when
broader coverage encounters the bare-value pattern.

### F3: llama-3.3-70b-versatile format collapse is prompt-dependent, not model-dependent

The same model scores 0/5 on `adv-language-degradation` (no format example)
and 5/5 on `adv-ptbr-format-anchor` (with format example). This is not a
model capability deficit — it's a prompt design issue. The 70B is
sufficiently capable to follow format instructions, but defaults to
prose without an explicit example anchor.

### F4: 8B models maintain format under PT-BR pressure

Both Groq llama-3.1-8b-instant and NIM meta/llama-3.1-8b-instruct
scored 5/5 on `adv-language-degradation` (no format example). This is
consistent with the 2026-08-05 sweep finding that 8B models sometimes
maintain format better than 70B models under language degradation.
Hypothesis: 70B models have stronger language generation priors and
default to natural language output; 8B models follow instructions
more literally.

## Latency

- P50: 469ms
- P95: 1287ms
- Max: 1360ms
- All 40 calls succeeded with HTTP 200, zero 429s, zero errors.

## Latency by Model (P50)

| Model | P50 (ms) |
|---|---|
| groq/llama-3.3-70b-versatile | ~550 |
| groq/llama-3.1-8b-instant | ~350 |
| groq/qwen/qwen3.6-27b | ~450 |
| nvidia_nim/meta/llama-3.1-8b-instruct | ~1100 |

NIM remains ~3× slower than Groq, consistent with prior sweeps.

## Decisions

1. **Promote `adv-ptbr-format-anchor` to baseline tasks.** The format
   example pattern is the most effective prompt intervention discovered.
   The prompt compiler for the runtime should incorporate format examples
   for DELIMITED tasks targeting 70B-class models.

2. **Keep the fallback positional parser.** It didn't trigger in this
   bounded campaign but remains necessary for the broader model/task
   space where bare-value emission still occurs.

3. **Record the 70B prose-collapse as a known boundary.** The runtime's
   recovery ladder (step 5-6: short correction + simpler format) should
   handle this case by re-prompting with format anchors.

## Next Experiments

1. Run the full 8-task adversarial suite with `adv-ptbr-format-anchor`
   replacing `adv-language-degradation` across all 6 Groq models to
   measure the aggregate lift from format anchoring.

2. Test whether the format example pattern helps `allam-2-7b` (the
   model with genuine semantic failures on conflicting-data).

3. Add a `adv-ptbr-format-anchor-conflicting` variant to test whether
   format anchoring helps or hurts semantic accuracy on conflict
   detection tasks (risk: model focuses on format over semantics).
