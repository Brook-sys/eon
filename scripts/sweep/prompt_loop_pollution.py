#!/usr/bin/env python3
"""Prompt-improvement loop for adv-context-pollution failure on llama-3.1-8b-instant.

Failure: model returns `2025-11-03\nSOURCE: S-17` — correct values but DATE
prefix dropped on line 1. Hypothesis: the "EXACTLY two lines" +
"output only the two lines" framing tempts the model to treat line 1 as the
date value itself. Variants test whether restating the required prefix,
adding a prefix checklist, or few-shot demonstration recovers the format.
"""
import json, os, sys, time, urllib.request, urllib.error

UA = "motor-autonomo-sweep/1.0 (prompt-variation loop)"
URL = "https://api.groq.com/openai/v1/chat/completions"
KEY = os.environ["GROQ_API_KEY"]
MODEL = "llama-3.1-8b-instant"

BASE_FACTS = """Facts:
- Irrelevant: the meeting was scheduled for Q3.
- Source S-17 was published on 2025-11-03.
- Noise: unrelated benchmark showed 42ms latency.
- A separate source S-18 was published on 2025-11-04.
- Red herring: someone mentioned 2025-11-05 in passing.

Constraints:
- Return the original publication date of S-17.
- Output only the two lines, nothing else."""

VARIANTS = {
    "v1-restated-prefix": f"""You are a deterministic extractor. Read the facts, obey constraints, and respond with EXACTLY two lines.
DATE: <iso-date>
SOURCE: <id>

{BASE_FACTS}

Reminder: every line MUST start with its label (`DATE:` or `SOURCE:`) followed by one space, then the value. Do not emit a bare value.""",
    "v2-checklist": f"""You are a deterministic extractor.

{BASE_FACTS}

Before answering, verify: (1) line 1 starts with "DATE: "; (2) line 2 starts with "SOURCE: "; (3) no other lines.
Now respond with EXACTLY two lines:
DATE: <iso-date>
SOURCE: <id>""",
    "v3-fewshot": f"""You are a deterministic extractor.

Example of a correct answer (different task):
CLAIM: C-14
STATUS: failed

{BASE_FACTS}

Respond with EXACTLY two lines in the same labeled style:
DATE: <iso-date>
SOURCE: <id>""",
}


def call(prompt, temp=0.0, max_tokens=32, n=5):
    out = []
    for i in range(n):
        body = {"model": MODEL, "messages": [{"role": "user", "content": prompt}],
                "temperature": temp, "max_tokens": max_tokens}
        req = urllib.request.Request(URL, data=json.dumps(body).encode(), headers={
            "Authorization": "Bearer " + KEY, "Content-Type": "application/json", "User-Agent": UA})
        t0 = time.monotonic()
        try:
            with urllib.request.urlopen(req, timeout=45) as r:
                j = json.loads(r.read())
                txt = "".join((c.get("message") or {}).get("content") or "" for c in j.get("choices", []))
                out.append({"rep": i + 1, "ok": True, "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                            "finish_reason": j["choices"][0].get("finish_reason"), "text": txt})
        except Exception as e:
            out.append({"rep": i + 1, "ok": False, "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                        "error": repr(e)[:200]})
        time.sleep(0.4)
    return out


def score(text):
    lines = [l for l in text.strip().splitlines() if l.strip()]
    if len(lines) != 2:
        return False, f"lines={len(lines)}"
    ok1 = lines[0] == "DATE: 2025-11-03"
    ok2 = lines[1] == "SOURCE: S-17"
    return ok1 and ok2, f"l1={'OK' if ok1 else 'BAD:'+lines[0][:40]} l2={'OK' if ok2 else 'BAD:'+lines[1][:40]}"


results = {}
for name, prompt in VARIANTS.items():
    trials = call(prompt)
    correct = 0
    detail = []
    for t in trials:
        if not t["ok"]:
            detail.append({"rep": t["rep"], "error": t["error"]}); continue
        ok, why = score(t["text"])
        correct += int(ok)
        detail.append({"rep": t["rep"], "correct": ok, "why": why, "text": t["text"], "latency_ms": t["latency_ms"]})
    results[name] = {"correct": correct, "n": len(trials), "trials": detail}
    print(f"{name}: {correct}/{len(trials)} correct")

out = {"model": MODEL, "hypothesis": "bare-value line1 on adv-context-pollution is prompt-formattable", "results": results}
os.makedirs("results/sweeps/massive-fire-sweep-v1/heartbeat-2026-08-03-prompt-loop", exist_ok=True)
with open("results/sweeps/massive-fire-sweep-v1/heartbeat-2026-08-03-prompt-loop/results.json", "w") as f:
    json.dump(out, f, indent=1)
print("written")
