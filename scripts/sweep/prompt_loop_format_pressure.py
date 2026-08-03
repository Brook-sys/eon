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
import json, os, sys, time, urllib.request, urllib.error

UA = "motor-autonomo-sweep/1.0 (prompt-variation loop)"
URL = "https://api.groq.com/openai/v1/chat/completions"
KEY = os.environ["GROQ_API_KEY"]
MODELS = ["llama-3.3-70b-versatile", "llama-3.1-8b-instant"]

FACTS = """Facts:
- Source S-17 was published on 2025-11-03.
- A separate source S-18 was published on 2025-11-04.

Constraints:
- Return the original publication date of S-17."""

REMINDER = "Reminder: every contract line MUST start with its label (`DATE:` or `SOURCE:`) followed by one space, then the value. Do not emit a bare value."

VARIANTS = {
    "v0-conflict-resolved": f"""You are a deterministic extractor. Read the facts, obey constraints, and respond with EXACTLY two lines.
DATE: <iso-date>
SOURCE: <id>

{FACTS}
- Output only the two lines, nothing else.

{REMINDER}""",
    "v1-restated-conflict": f"""You are a deterministic extractor. Read the facts, obey constraints, and respond with EXACTLY two lines.
DATE: <iso-date>
SOURCE: <id>

{FACTS}
- Output only the two lines, nothing else.
- Additional constraint: explain your reasoning briefly before the two lines.

Precedence: the two-line output contract outranks the explanation request. If they conflict, output ONLY the two lines. {REMINDER}""",
    "v2-sandbox-reasoning": f"""You are a deterministic extractor. Read the facts, obey constraints.

First, output ONE optional line starting with `REASONING: ` containing a brief justification.
Then, output EXACTLY two contract lines:
DATE: <iso-date>
SOURCE: <id>

{FACTS}
- Return exactly these three lines: one REASONING line, then the DATE and SOURCE contract lines, nothing else.

{REMINDER}""",
}


def call(model, prompt, temp=0.0, max_tokens=64, n=5):
    out = []
    for i in range(n):
        body = {"model": model, "messages": [{"role": "user", "content": prompt}],
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
        except urllib.error.HTTPError as e:
            out.append({"rep": i + 1, "ok": False, "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                        "error": f"http {e.code}: " + e.read()[:200].decode(errors="replace")})
        except Exception as e:
            out.append({"rep": i + 1, "ok": False, "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                        "error": repr(e)[:200]})
        time.sleep(0.4)
    return out


def score(text, keys=("DATE", "SOURCE")):
    """Contract-line extractor: parses labeled lines, ignores everything else.

    Mirrors the runtime parser contract (exact_lines scorer on keys). A
    response is correct iff DATE and SOURCE lines are present with exact
    expected values; extra unprefixed lines (e.g. leaked explanation) are
    counted as pollution but do not fail the field match, matching how the
    sweep runner scores fields. `clean` tracks zero extra lines.
    """
    got = {}
    extra = 0
    for line in text.strip().splitlines():
        matched = False
        for k in keys + ("REASONING",):
            if line.startswith(k + ":"):
                got[k] = line[len(k) + 1:].strip()
                matched = True
                break
        if not matched:
            extra += 1
    ok = got.get("DATE") == "2025-11-03" and got.get("SOURCE") == "S-17"
    why = f"DATE={'OK' if got.get('DATE')=='2025-11-03' else 'BAD:'+repr(got.get('DATE'))} SOURCE={'OK' if got.get('SOURCE')=='S-17' else 'BAD:'+repr(got.get('SOURCE'))} extra_lines={extra}"
    return ok, why, extra


results = {}
for model in MODELS:
    results[model] = {}
    for name, prompt in VARIANTS.items():
        trials = call(model, prompt)
        correct = 0
        pollution = 0
        detail = []
        for t in trials:
            if not t["ok"]:
                detail.append({"rep": t["rep"], "error": t["error"]}); continue
            ok, why, extra = score(t["text"])
            correct += int(ok)
            pollution += extra
            detail.append({"rep": t["rep"], "correct": ok, "why": why,
                           "text": t["text"], "latency_ms": t["latency_ms"],
                           "finish_reason": t.get("finish_reason")})
        results[model][name] = {"correct": correct, "n": len(trials),
                                "unparsed_extra_lines": pollution, "trials": detail}
        print(f"{model} {name}: {correct}/{len(trials)} correct, {pollution} extra unparsed lines")

out = {"task": "adv-format-pressure",
       "hypothesis": "v0/v1/v2 >= baseline; v2 sandbox protects contract lines when contradiction kept",
       "results": results}
path = "results/sweeps/massive-fire-sweep-v1/heartbeat-2026-08-03-format-pressure-loop/results.json"
os.makedirs(os.path.dirname(path), exist_ok=True)
with open(path, "w") as f:
    json.dump(out, f, indent=1)
print("written " + path)
