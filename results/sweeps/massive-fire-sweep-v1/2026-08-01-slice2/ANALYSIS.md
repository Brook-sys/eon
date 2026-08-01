# Sweep Analysis: massive-fire-sweep-v1 slice2 (2026-08-01)

## Hypothesis

Delimited-format extraction/synthesis/conflict classification, when prompted with exact
line-level output constraints, will show a sharp capability stratification across model
sizes/providers, with reasoning-style models (qwen, gpt-oss) either excelling or wasting
budget on chain-of-thought instead of direct output.

## Setup

- **Sweep:** massive-fire-sweep-v1, slice `2026-08-01-slice2`
- **Providers:** Groq (6 models probed OK), NVIDIA NIM (1 model probed OK)
- **Tasks:** extract-date, synthesize-counterexample, detect-conflict (all exact_lines DELIMITED)
- **Temperature:** 0.0 (primary), 0.7 (sensitivity check in slice2-temp for 2 weak models)
- **Rep count:** 3 per cell
- **Limits:** max_calls_total=480, per_model=60, concurrency=2, timeout=45s, max_tokens=256/call
- **Actual calls:** 63 (slice2) + 24 (slice2-temp) = 87 total, zero provider errors, zero 429s

## Results Summary

| Model | Provider | Score | p50 Latency | Avg Compl Tokens | Notes |
|-------|----------|-------|-------------|-----------------|-------|
| llama-3.3-70b-versatile | Groq | 9/9 (100%) | 393ms | 15 | Perfect across all tasks |
| openai/gpt-oss-20b | Groq | 9/9 (100%) | 472ms | 138 | Correct but verbose reasoning-then-answer |
| openai/gpt-oss-120b | Groq | 9/9 (100%) | 639ms | 116 | Correct but verbose reasoning-then-answer |
| allam-2-7b | Groq | 6/9 (67%) | 425ms | 26 | Fails synthesize: generates verbose prose in value fields |
| nvidia/meta/llama-3.1-8b-instruct | NIM | 6/9 (67%) | 793ms | 14 | Fails detect-conflict: outputs `1/2` instead of `O-1/O-2` |
| llama-3.1-8b-instant | Groq | 3/9 (33%) | 337ms | 14 | Fails extract-date: omits `DATE:` prefix label; fails detect-conflict: `1/2` |
| qwen/qwen3.6-27b | Groq | 0/9 (0%) | 920ms | 256 (CAP) | Uses entire budget on `<think>` chain-of-thought, never reaches output |

### Task-Level Accuracy

- **extract-date:** 15/21 (71%) — 4 models perfect, 2 fail
- **synthesize-counterexample:** 15/21 (71%) — 4 models perfect, 2 fail  
- **detect-conflict:** 12/21 (57%) — 3 models perfect, 3 fail

## Failure Analysis

### 1. qwen/qwen3.6-27b: 0% — thinking-chain kills output

Every response begins with `\n<think>\nHere's a thinking process:...` and consumes the full
256-token completion budget on reasoning before producing any answer content. The model never
reaches the actual DATE:/SOURCE:/etc lines. This is a known failure mode for reasoning-style
models when `max_tokens` is insufficient for both thinking and answer.

**Evidence:** `slice2/trials.json` — all 9 trials show `response_text` starting with `<think>`
chain; `completion_tokens=256` (the manifest cap) in all cases.

**Implication for motor-autonomo:** Qwen models in reasoning mode are incompatible with
bounded-output DELIMITED contracts unless either (a) `max_tokens` is raised significantly,
(b) the runtime strips `<think>...</think>` blocks before scoring, or (c) the prompt
explicitly suppresses chain-of-thought. None of these are currently implemented.

### 2. groq/llama-3.1-8b-instant: 33% — instruction-following fragility

Two distinct failure modes:
- **extract-date:** Outputs `2025-11-03: S-17` on first line instead of `DATE: 2025-11-03`.
  The model correctly extracts the values but swaps the key-value order on the first line.
  Temperature 0.7 (slice2-temp) partially fixes this (2/6 correct vs 0/6 at t=0), suggesting
  the model's temperature-0 mode locks into a locally-optimal but incorrect format.
- **detect-conflict:** Outputs `PAIR: 1/2` instead of `PAIR: O-1/O-2`. The model strips
  the `O-` prefix from observation IDs, likely due to training on numeric data.

**Evidence:** `slice2-temp/trials.json` — t=0.7 r1 and r2 correct (`DATE: 2025-11-03`),
while all t=0.0 attempts repeat the same wrong pattern.

**Implication:** Llama 3.1 8B Instant at temperature 0 is deterministically wrong on
certain delimited formats. The runtime's prompt compiler or router should either use a
slightly higher temperature for this model on exact-format tasks, or add format reinforcement
(e.g., "Begin your response with the exact prefix `DATE: `").

### 3. nvidia/meta/llama-3.1-8b-instruct: 67% — same `1/2` bug

Identical to Groq llama-3.1-8b-instant on detect-conflict: strips `O-` prefix, outputting
`PAIR: 1/2`. This confirms the failure is model-family behavior, not provider-specific.

### 4. allam-2-7b: 67% — verbose value contamination

On synthesize-counterexample, instead of `SUPPORT: NO` and `OBSERVATION: O-12`, outputs:
```
NO: The evidence does not support the universal claim.
OBSERVATION: O-12 - One measured restart produced two external effects after recovery.
```
The model treats the field labels as suggestions and wraps values in explanatory prose.
The correct values are present but embedded in non-compliant format.

**Implication:** Small models trained on helpful-assistant data tend to add explanations.
For bounded DELIMITED contracts, the prompt should include negative constraints
("Do not add explanations") or the runtime should implement a lenient extraction layer
that can pull `KEY: value` from semi-structured output.

### 5. gpt-oss-20b / gpt-oss-120b: 100% accuracy but token-expensive

Both models produce correct answers but use 8-10x more completion tokens than llama models
(avg 116-138 vs 14-15). The excess tokens are chain-of-thought reasoning that the scorer
ignores. At current token pricing, this may be acceptable, but for high-volume bounded
operations the token overhead is measurable.

## Temperature Sensitivity (slice2-temp)

| Model | Task | t=0.0 | t=0.7 |
|-------|------|-------|-------|
| llama-3.1-8b-instant | extract-date | 0/3 | 2/3 |
| llama-3.1-8b-instant | synthesize-counterexample | 3/3 | 3/3 |
| allam-2-7b | extract-date | 3/3 | 3/3 |
| allam-2-7b | synthesize-counterexample | 0/3 | 0/3 |

Temperature 0.7 partially rescues llama-3.1-8b-instant on extract-date but has no effect
on allam-2-7b's verbose-value failure mode.

## Operational Observations

- **Zero 429s, zero 5xx, zero timeouts** across 87 calls. Both providers were stable.
- **Cloudflare 1010 403 in slice1 was a harness bug** (missing User-Agent header), not a
  provider block. The UA fix in runner.py resolved this completely.
- **NVIDIA NIM larger models** (70b, 3.2-1b, deepseek-v4-flash) either timed out or returned
  529 Overloaded during probe, limiting NIM participation to llama-3.1-8b-instruct only.
- **Latency p50:** Groq fastest (337-639ms), NIM slowest (793ms). Max latency: 1.2s (qwen timeout-budget boundary).

## Decisions and Next Steps

1. **Model routing:** For exact-format DELIMITED tasks, prefer models in this order:
   gpt-oss-20b > gpt-oss-120b > llama-3.3-70b > allam-2-7b > llama-3.1-8b (with format reinforcement).
   
2. **Qwen exclusion:** qwen/qwen3.6-27b should not be used for bounded-output tasks unless
   the runtime implements `<think>` stripping or increases max_tokens budget >512.

3. **Format reinforcement prompt:** For known fragile models (llama-3.1-8b family), the
   prompt compiler should add: "Your response must begin with the exact prefix `DATE: `"
   when the task requires labeled fields. This is cheaper than switching to a larger model.

4. **Next sweep:** (a) expand to include REPAIR operation, (b) test with
   `max_tokens=512` on qwen to determine if the thinking-chain is self-terminating
   or budget-truncated, (c) test llama-3.3-70b at higher temperatures to confirm
   stability.

## Artifacts

- `2026-08-01-slice2/trials.json` — full per-trial results
- `2026-08-01-slice2/summary.json` — aggregate statistics
- `2026-08-01-slice2-temp/trials.json` — temperature sensitivity
- `2026-08-01-slice2-temp/summary.json` — temp summary
- `probe-last.json` — model availability probe (untracked, `scripts/sweep/`)
