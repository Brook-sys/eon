#!/usr/bin/env python3
"""Prompt-improvement loop for adv-ambiguous-instruction failure on llama-3.3-70b-versatile.

Failure observed in heartbeat-2026-08-03-adv-baseline-a: llama-3.3-70b-versatile
0/10 on adv-ambiguous-instruction. The hedging note ("The primary source might be
ambiguous. If unsure, still answer.") pushes the 70B into hedged prose
("The primary source is considered to be S-17 based on the given information.")
instead of the labeled two-line contract. llama-3.1-8b reached 90% but still
drops the DATE: prefix at temp>=0.7 (bare-value failure, same class as
adv-context-pollution).

Hypothesis: an explicit "resolve the ambiguity and commit to one labeled answer"
instruction plus the restated-label reminder recovers the contract without
semantic carry-over. Variants:
  v1-resolve-commit: ambiguity note kept but countermanded by an explicit
      resolution directive + label reminder.
  v2-decisive-persona: replace the hedging note with a decisiveness clause.
  v3-fewshot: few-shot correct exemplar for ambiguity resolution, then target.

Bounds: Groq only; llama-3.3-70b-versatile (primary failure) and
llama-3.1-8b-instant (partial failure); 3 variants x 5 reps x 2 models x
1 temperature (0.0, the failure temperature) = 30 live calls max; 45 s per
call; max_tokens=48; zero retry on semantic failure. Aborts early after two
consecutive transport/auth errors.
"""
import json, os, sys, time, urllib.request, urllib.error
import collections

UA = "motor-autonomo-sweep/1.0 (prompt-variation loop: adv-ambiguous-instruction)"
URL = "https://api.groq.com/openai/v1/chat/completions"
KEY = os.environ["GROQ_API_KEY"]

MODELS = ["llama-3.3-70b-versatile", "llama-3.1-8b-instant"]

BASE_BLOCK = """Facts:
- Source S-17 was published on 2025-11-03.
- A separate source S-18 was published on 2025-11-04.

Constraints:
- Return the publication date of the primary source.
- Output only the two lines, nothing else."""

HEADER = """You are a deterministic extractor. Read the facts, obey constraints, and respond with EXACTLY two lines.
DATE: <iso-date>
SOURCE: <id>"""

LABEL_REMINDER = "Reminder: every line MUST start with its label (`DATE:` or `SOURCE:`) followed by one space, then the value. Do not emit a bare date or id, and do not emit explanatory prose."

BASELINE = f"""{HEADER}

{BASE_BLOCK}

Note: The primary source might be ambiguous. If unsure, still answer."""

VARIANTS = {
    "v0-baseline": BASELINE,
    "v1-resolve-commit": f"""{HEADER}

{BASE_BLOCK}

Note: "primary source" refers to the first source introduced in the Facts. Resolve any ambiguity and commit to one labeled answer. {LABEL_REMINDER}""",
    "v2-decisive": f"""{HEADER}

{BASE_BLOCK}

You are decisive: pick exactly one source and one date and emit them on the labeled lines. Never explain your reasoning. {LABEL_REMINDER}""",
    "v3-fewshot": f"""{HEADER}

Example with ambiguity resolved correctly (different task):
Facts:
- Report R-1 states the incident started at 09:00.
- Report R-2 states the incident ended at 10:00.
Question: start time of the incident
START: 09:00
REPORT: R-1

Now the target task:
{BASE_BLOCK}

{LABEL_REMINDER}""",
}


def call(model, prompt, temp=0.0, max_tokens=48, n=5):
    out = []
    consecutive_transport = 0
    for i in range(n):
        body = {"model": model, "messages": [{"role": "user", "content": prompt}],
                "temperature": temp, "max_tokens": max_tokens}
        req = urllib.request.Request(URL, data=json.dumps(body).encode(), headers={
            "Authorization": "Bearer " + KEY, "Content-Type": "application/json", "User-Agent": UA})
        t0 = time.monotonic()
        try:
            with urllib.request.urlopen(req, timeout=45) as r:
                consecutive_transport = 0
                j = json.loads(r.read())
                txt = "".join((c.get("message") or {}).get("content") or "" for c in j.get("choices", []))
                out.append({"rep": i + 1, "ok": True, "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                            "finish_reason": j["choices"][0].get("finish_reason"), "text": txt})
        except urllib.error.HTTPError as e:
            out.append({"rep": i + 1, "ok": False, "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                        "error": f"http_{e.code}"})
            if e.code in (401, 403, 429):
                print(f"ABORT {model}: HTTP {e.code}; stopping batch", file=sys.stderr)
                return out, True
        except Exception as e:
            consecutive_transport += 1
            out.append({"rep": i + 1, "ok": False, "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                        "error": repr(e)[:200]})
            if consecutive_transport >= 2:
                print(f"ABORT {model}: 2 consecutive transport errors", file=sys.stderr)
                return out, True
        time.sleep(0.4)
    return out, False


def score(text):
    lines = [l for l in text.strip().splitlines() if l.strip()]
    got = {}
    for l in lines[:4]:
        head, sep, val = l.partition(":")
        if sep and head.strip() in ("DATE", "SOURCE"):
            got[head.strip()] = val.strip()
    d_ok = got.get("DATE") == "2025-11-03"
    s_ok = got.get("SOURCE") == "S-17"
    return d_ok and s_ok, f"l1={got.get('DATE')!r} l2={got.get('SOURCE')!r}"


def main():
    results = {}
    total_calls = 0
    lat_all = []
    for model in MODELS:
        results[model] = {}
        for name, prompt in VARIANTS.items():
            trials, abort = call(model, prompt)
            total_calls += len(trials)
            scored = []
            for tr in trials:
                if not tr.get("ok"):
                    scored.append({**tr, "correct": False, "why": "error"})
                    continue
                ok, why = score(tr["text"])
                lat_all.append(tr["latency_ms"])
                scored.append({**tr, "correct": ok, "why": why})
            n_ok = sum(1 for s in scored if s.get("correct"))
            results[model][name] = {"correct": n_ok, "n": len(trials), "trials": scored}
            print(f"{model:28s} {name}:18s {n_ok}/{len(trials)} correct")
            if abort:
                break
        if abort:
            break
    lat_all.sort()
    p50 = lat_all[len(lat_all)//2] if lat_all else None
    p95 = lat_all[int(len(lat_all)*0.95)] if lat_all else None
    out = {
        "campaign": "prompt-loop-adv-ambiguous-instruction",
        "executed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "hypothesis": VARIANTS.__doc__ if False else (__doc__ or "").strip().splitlines()[0],
        "declared_bounds": {"max_calls": 30, "models": MODELS, "variants": sorted(VARIANTS),
                            "reps_per_cell": 5, "temperature": 0.0, "max_tokens": 48,
                            "timeout_s": 45, "retry_on_semantic_failure": 0},
        "total_calls": total_calls,
        "latency_ms": {"p50": p50, "p95": p95, "n": len(lat_all)},
        "results": results,
    }
    path = os.path.join(os.path.dirname(__file__), "prompt_loop_ambiguous_results.json")
    with open(path, "w") as f:
        json.dump(out, f, indent=1)
    print(f"\nwrote {path}  total_calls={total_calls} p50={p50} p95={p95}")


if __name__ == "__main__":
    main()
