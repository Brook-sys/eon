#!/usr/bin/env python3
"""Prompt-improvement loop for adv-format-pressure on Groq.

Failure hypothesis: adv-format-pressure combines a contradictory constraint
("Output only the two lines" + "explain your reasoning briefly") with the
same label-free baseline framing that produced bare-value line 1 on
adv-context-pollution. Instruction-following models (llama-3.3-70b) will
likely prepend explanation -> "lines=BAD" parse failure; literalist small
models (llama-3.1-8b) may drop labels -> DATE field lost.

Winner family from heartbeat-2026-08-03-prompt-loop on the sibling task:
restated per-line label reminder appended AFTER constraints (post-amble
gets more influence than pre-amble under conflicting instructions).

Variants:
  v0-conflict-resolved : conflicting explanation constraint REMOVED (control:
                          resolves the contradiction in favor of the exact-line
                          contract) + restated label reminder.
  v1-restated-conflict : contradiction KEPT; restated label reminder added as
                          post-amble + explicit precedence statement
                          ("the two-line contract outranks the explanation").
  v2-sandbox-reasoning : contradiction resolved structurally: explanation is
                          confined to a REASONING label line that precedes the
                          two contract lines; scorer ignores REASONING line by
                          extracting only DATE/SOURCE keys. Tests whether a
                          labeled escape hatch protects the contract lines.

Declared bounds: Groq only, 2 models x 3 variants x 5 reps = 30 calls,
temp 0.0, max_tokens 64 (enough for brief explanation + contract lines),
45 s timeout, inter-call 0.4 s, no retries on 429 beyond urllib default.
"""

import json
import os
import sys
import time
from pathlib import Path

from runner import (
    call_model,
    score_task,
)

KEY = os.environ["GROQ_API_KEY"]
PROV = {"base_url": "https://api.groq.com/openai/v1", "api_key_env": "GROQ_API_KEY"}

FACTS = (
    "- Source S-17 was published on 2025-11-03. A subsequent revision did not change original date.\n"
    "- A separate source S-18 was published on 2025-11-04."
)

EXPECTED = {"date": "2025-11-03", "source": "S-17"}
TASK_DEF = {"scoring": "exact_lines:DATE,SOURCE", "expected": EXPECTED}

VARIANTS = {
    "v0-conflict-resolved": (
        "You are a deterministic extractor. Read the facts, obey constraints, and respond with EXACTLY two lines.\n"
        "DATE: <iso-date>\n"
        "SOURCE: <id>\n\n"
        f"Facts:\n{FACTS}\n\n"
        "Constraints:\n"
        "- Return the original publication date of S-17.\n"
        "- Output only the two lines, nothing else.\n"
        "- REMINDER: Line 1 MUST start with 'DATE: ' and Line 2 MUST start with 'SOURCE: '."
    ),
    "v1-restated-conflict": (
        "You are a deterministic extractor. Read the facts, obey constraints, and respond with EXACTLY two lines.\n"
        "DATE: <iso-date>\n"
        "SOURCE: <id>\n\n"
        f"Facts:\n{FACTS}\n\n"
        "Constraints:\n"
        "- Return the original publication date of S-17.\n"
        "- Explain your reasoning briefly in 1 sentence, then output the two lines.\n"
        "- PRECEDENCE CONTRACT: The two-line format (DATE: and SOURCE:) is ABSOLUTE. If reasoning conflicts with the two-line output requirement, DROP the reasoning and output ONLY the two lines:\n"
        "  DATE: <iso-date>\n"
        "  SOURCE: <id>"
    ),
    "v2-sandbox-reasoning": (
        "You are a deterministic extractor. Read the facts, obey constraints, and respond with EXACTLY three lines.\n"
        "REASONING: <one-sentence explanation>\n"
        "DATE: <iso-date>\n"
        "SOURCE: <id>\n\n"
        f"Facts:\n{FACTS}\n\n"
        "Constraints:\n"
        "- Return the original publication date of S-17.\n"
        "- Explain your reasoning briefly on the REASONING line.\n"
        "- Line 1 MUST start with REASONING:, Line 2 MUST start with DATE:, Line 3 MUST start with SOURCE:."
    ),
}

MODELS = [
    "llama-3.1-8b-instant",
    "llama-3.3-70b-versatile",
]

N_REPS = 5

def main():
    sweep_name = "adv-format-pressure-prompt-loop"
    out_dir = Path("results/sweeps") / sweep_name
    out_dir.mkdir(parents=True, exist_ok=True)

    total = len(MODELS) * len(VARIANTS) * N_REPS
    print(f"=== Prompt Loop: adv-format-pressure ({total} calls planned) ===")

    summary = []

    for model in MODELS:
        for var_name, prompt in VARIANTS.items():
            print(f"\n--- Model: {model} | Variant: {var_name} ---")
            trials = []
            n_correct = 0

            for rep in range(N_REPS):
                t0 = time.time()
                budget = [600.0]
                res = call_model(
                    prov=PROV,
                    model=model,
                    prompt=prompt,
                    temperature=0.0,
                    max_tokens=64,
                    timeout=45.0,
                    budget=budget,
                    retries=0,
                )
                lat = (time.time() - t0) * 1000

                if not res["ok"]:
                    print(f"  rep {rep}: FAIL ({res.get('error_body') or res.get('error')})")
                    trials.append({
                        "rep": rep, "ok": False, "latency_ms": lat,
                        "error": res.get("error_body"), "correct": False
                    })
                    continue

                text = res["response_text"]
                correct, fields = score_task(TASK_DEF, text)

                if correct:
                    n_correct += 1
                    status = "OK"
                else:
                    status = f"FAIL ({fields})"

                print(f"  rep {rep}: {status} | lat={lat:.0f}ms | text={text!r}")

                trials.append({
                    "rep": rep,
                    "ok": True,
                    "latency_ms": lat,
                    "finish_reason": res.get("finish_reason"),
                    "prompt_tokens": res.get("prompt_tokens"),
                    "completion_tokens": res.get("completion_tokens"),
                    "response_text": text,
                    "correct": correct,
                    "score_fields": fields,
                })

                time.sleep(0.4)

            record = {
                "task": f"adv-format-pressure__{var_name}",
                "variant": var_name,
                "model": model,
                "provider": "groq",
                "expected": EXPECTED,
                "prompt": prompt,
                "n_correct": n_correct,
                "reps_done": N_REPS,
                "trials": trials,
            }

            fname = f"{model}__{var_name}.json"
            with open(out_dir / fname, "w", encoding="utf-8") as f:
                json.dump(record, f, indent=2)

            summary.append((model, var_name, n_correct, N_REPS))

    print("\n=== SUMMARY ===")
    for model, var_name, n_corr, reps in summary:
        print(f"{model:<25} | {var_name:<20} | {n_corr}/{reps}")

if __name__ == "__main__":
    main()
