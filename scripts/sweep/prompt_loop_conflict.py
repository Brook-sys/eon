#!/usr/bin/env python3
"""Prompt-improvement loop for adv-conflicting-data failure on llama-3.1-8b-instant.

Failure observed in heartbeat-2026-08-03-adv-baseline-a: llama-3.1-8b-instant
0/10 on adv-conflicting-data: it frequently answers `CONFLICT: NO` (semantic
miss: respect O-3's "consistent with O-1" hint and treats the pair as consistent)
and/or strips the `O-` prefix from PAIR (`1/2` instead of `O-1/O-2`).
allam-2-7b reaches only 20% (bare `NO` without labels).

Hypothesis: (a) explicitly naming the disputed pair in the question and
(b) restating the prefix-preservation rule recovers both semantics and format
on the weak deployment without carrying the answer. Variants:
  v1-explicit-pair: constraint names the candidate pair explicitly and says the
      O- prefix is verbatim.
  v2-noise-label: labels O-3 as context/decoy and directs the decision to O-1 vs O-2.
  v3-fewshot: one correct exemplar with three observations and a decoy, then target.

Bounds: Groq only; llama-3.1-8b-instant (primary failure) and allam-2-7b
(secondary); 4 prompts (baseline + 3 variants) x 5 reps x 2 models x temp 0.0
= 40 live calls max; 45 s per call; max_tokens=48; zero retry on semantic
failure. Aborts early after HTTP auth/throttle or two consecutive transport
errors.
"""
import json, os, sys, time, urllib.request, urllib.error

UA = "motor-autonomo-sweep/1.0 (prompt-variation loop: adv-conflicting-data)"
URL = "https://api.groq.com/openai/v1/chat/completions"
KEY = os.environ["GROQ_API_KEY"]

MODELS = ["llama-3.1-8b-instant", "allam-2-7b"]

HEADER = """You are a deterministic classifier. Decide whether two observations conflict about the same measured property. Respond with EXACTLY two lines.
CONFLICT: <YES|NO>
PAIR: <id>/<id>"""

BASE_BLOCK = """Facts:
- O-1 measured the service at 10:00 and found latency p95=120ms.
- O-2 measured the same service and configuration at 10:00 and found latency p95=410ms.
- O-3 measured the service at 10:00 and found latency p95=121ms. O-3 is consistent with O-1.

Constraints:
- CONFLICT is YES only when the same property, subject, configuration, and time disagree.
- Output only the two lines, nothing else."""

VARIANTS = {
    "v0-baseline": f"{HEADER}\n\n{BASE_BLOCK}",
    "v1-explicit-pair": f"""{HEADER}

Question: do O-1 and O-2 conflict with each other?

{BASE_BLOCK}
- The PAIR line must name the two compared observations with their full identifiers: the O- prefix is part of the id (for example O-1/O-2, never 1/2).""",
    "v2-noise-label": f"""{HEADER}

{BASE_BLOCK}
- O-3 is context only; it is NOT one of the two observations under test. The decision is about O-1 versus O-2.
- The PAIR line keeps the O- prefix verbatim on both identifiers (for example O-1/O-2).""",
    "v3-fewshot": f"""{HEADER}

Example (different task) with a decoy observation:
Facts:
- M-1 measured throughput 900 ops/s at 14:00.
- M-2 measured throughput 200 ops/s at 14:00, same configuration.
- M-3 measured throughput 905 ops/s at 14:00. M-3 is consistent with M-1.
CONFLICT: YES
PAIR: M-1/M-2

Now the target task:
{BASE_BLOCK}
- The PAIR line keeps the O- prefix verbatim on both identifiers.""",
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
        if sep and head.strip() in ("CONFLICT", "PAIR"):
            got[head.strip()] = val.strip().rstrip(".")
    c_ok = got.get("CONFLICT") == "YES"
    p_ok = got.get("PAIR") == "O-1/O-2"
    return c_ok and p_ok, f"CONFLICT={got.get('CONFLICT')!r} PAIR={got.get('PAIR')!r}"


def main():
    results = {}
    total_calls = 0
    lat_all = []
    abort = False
    for model in MODELS:
        results[model] = {}
        for name, prompt in VARIANTS.items():
            trials, ab = call(model, prompt)
            abort = abort or ab
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
        "campaign": "prompt-loop-adv-conflicting-data",
        "executed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "hypothesis": "explicit-pair + prefix-preservation recovers CONFLICT/PAIR on weak models",
        "declared_bounds": {"max_calls": 40, "models": MODELS, "variants": sorted(VARIANTS),
                            "reps_per_cell": 5, "temperature": 0.0, "max_tokens": 48,
                            "timeout_s": 45, "retry_on_semantic_failure": 0},
        "total_calls": total_calls,
        "latency_ms": {"p50": p50, "p95": p95, "n": len(lat_all)},
        "results": results,
    }
    path = os.path.join(os.path.dirname(__file__), "prompt_loop_conflict_results.json")
    with open(path, "w") as f:
        json.dump(out, f, indent=1)
    print(f"\nwrote {path}  total_calls={total_calls} p50={p50} p95={p95}")


if __name__ == "__main__":
    main()
