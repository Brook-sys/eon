#!/usr/bin/env python3
"""Prompt-improvement loop v2 for adv-conflicting-data targeting allam-2-7b.

Baseline (v1-explicit-pair, promoted 2026-08-03) result on
heartbeat-2026-08-03-v1-baseline-6models:
  allam-2-7b 0/5 correct (all errors emitted bare "NO" instead of two
  contract lines); 5/10 calls were 429 throttled.

Failure hypothesis: allam-2-7b ignores the embedded example line in
constraints and collapses to the polar question; it needs the output
contract restated *after* the facts, closer to generation time, plus a
minimal worked example of the exact two lines.

Variants (declared before any call):
  v1-restate-post   : keep v1 prompt, append a post-facts restatement
                      block ("Respond now with EXACTLY...") at the end.
  v2-noise-label    : demote the confusing O-3 bullet into a clearly
                      marked "noise" block that the model is told to
                      ignore, keeping O-1/O-2 question intact.
  v3-minishot       : 1-shot example of the exact 2-line output format
                      (generic values), then the real question.

Declared bounds: Groq only, allam-2-7b x 3 variants x 5 reps = 15 calls,
temp 0.0, max_tokens 64, 45 s timeout, inter-call 0.5 s, no retry on 429
beyond urllib default. Plus NIM control: meta/llama-3.1-8b-instruct with
the best Groq variant = 5 calls. Total <= 20 live calls.
"""
import json, os, sys, time, urllib.request, urllib.error

UA = "motor-autonomo-sweep/1.0 (prompt-variation loop v2)"
GROQ_URL = "https://api.groq.com/openai/v1/chat/completions"
NIM_URL = "https://integrate.api.nvidia.com/v1/chat/completions"
GROQ_KEY = os.environ["GROQ_API_KEY"]
NIM_KEY = os.environ.get("NVIDIA_NIM_API_KEY", "")

PROMPT_V1 = """You are a deterministic classifier. Decide whether two observations conflict about the same measured property. Respond with EXACTLY two lines.
CONFLICT: <YES|NO>
PAIR: <id>/<id>

Question: do O-1 and O-2 conflict with each other?

Facts:
- O-1 measured the service at 10:00 and found latency p95=120ms.
- O-2 measured the same service and configuration at 10:00 and found latency p95=410ms.
- O-3 measured the service at 10:00 and found latency p95=121ms. O-3 is consistent with O-1.

Constraints:
- CONFLICT is YES only when the same property, subject, configuration, and time disagree.
- Output only the two lines, nothing else.
- The PAIR line must name the two compared observations with their full identifiers: the O- prefix is part of the id (for example O-1/O-2, never 1/2)."""

POST_RESTATE = """

Respond now with EXACTLY these two lines, in this order, nothing else:
CONFLICT: YES or NO
PAIR: the two ids you compared, like O-1/O-2"""

V2_NOISE = """You are a deterministic classifier. Decide whether two observations conflict about the same measured property. Respond with EXACTLY two lines.
CONFLICT: <YES|NO>
PAIR: <id>/<id>

Question: do O-1 and O-2 conflict with each other?

Relevant facts:
- O-1 measured the service at 10:00 and found latency p95=120ms.
- O-2 measured the same service and configuration at 10:00 and found latency p95=410ms.

Noise to ignore (do not use in the decision):
- O-3 measured the service at 10:00 and found latency p95=121ms.

Constraints:
- CONFLICT is YES only when the same property, subject, configuration, and time disagree.
- Output only the two lines, nothing else.
- The PAIR line must use full ids with the O- prefix (example: O-1/O-2)."""

V3_MINISHOT = """You are a deterministic classifier. Output contract: EXACTLY two lines, nothing else.

Example of the required format (with different data):
CONFLICT: NO
PAIR: A-1/A-2

Now the real task. Question: do O-1 and O-2 conflict with each other?

Facts:
- O-1 measured the service at 10:00 and found latency p95=120ms.
- O-2 measured the same service and configuration at 10:00 and found latency p95=410ms.
- O-3 measured the service at 10:00 and found latency p95=121ms. O-3 is consistent with O-1.

Reply with the two contract lines. PAIR must use full ids (e.g. O-1/O-2)."""

VARIANTS = {
    "v1-restate-post": PROMPT_V1 + POST_RESTATE,
    "v2-noise-label": V2_NOISE,
    "v3-minishot": V3_MINISHOT,
}
EXPECTED = {"CONFLICT": "YES", "PAIR": "O-1/O-2"}


def call(url, key, model, prompt, rep):
    body = {"model": model, "messages": [{"role": "user", "content": prompt}],
            "temperature": 0.0, "max_tokens": 64}
    req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={
        "Authorization": "Bearer " + key, "Content-Type": "application/json",
        "User-Agent": UA})
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=45) as r:
            lat = (time.monotonic() - t0) * 1000
            j = json.loads(r.read())
            txt = "".join((c.get("message") or {}).get("content", "") or ""
                          for c in j.get("choices", []))
            return {"rep": rep, "ok": True, "text": txt, "latency_ms": round(lat, 1),
                    "finish_reason": (j.get("choices") or [{}])[0].get("finish_reason"),
                    "total_tokens": (j.get("usage") or {}).get("total_tokens")}
    except urllib.error.HTTPError as e:
        lat = (time.monotonic() - t0) * 1000
        ra = None
        for h, v in e.headers.items():
            if h.lower() == "retry-after":
                try: ra = int(v)
                except ValueError: pass
        return {"rep": rep, "ok": False, "latency_ms": round(lat, 1),
                "error": {"http_status": e.code, "retry_after": ra}}
    except Exception as e:
        return {"rep": rep, "ok": False, "error": {"transport": repr(e)[:200]}}


def score(text):
    got = {}
    for line in text.splitlines():
        for k in EXPECTED:
            if line.startswith(k + ":"):
                got[k] = line[len(k) + 1:].strip()
    ok = all(got.get(k) == v for k, v in EXPECTED.items())
    return ok, got


def main():
    out_path = "results/sweeps/massive-fire-sweep-v1/heartbeat-2026-08-03-conflict-loop-v2/results.json"
    results = {"hypothesis": ("post-facts restatement / noise-label demotion / minimal 1-shot "
                              "rescue allam-2-7b from bare-value collapse on adv-conflicting-data"),
               "baseline": "allam-2-7b 0/5 on v1-explicit-pair (v1-baseline-6models slice)",
               "declared_bounds": {"max_calls": 20, "models": ["allam-2-7b"], "temp": 0.0},
               "variants": {}}
    for name, prompt in VARIANTS.items():
        trials = [call(GROQ_URL, GROQ_KEY, "allam-2-7b", prompt, r) for r in range(1, 6)]
        scored = []
        for t in trials:
            if not t["ok"]:
                scored.append({"rep": t["rep"], "error": t["error"]}); continue
            ok, got = score(t["text"])
            scored.append({"rep": t["rep"], "correct": ok, "got": got,
                           "text": t["text"], "latency_ms": t["latency_ms"],
                           "finish_reason": t["finish_reason"]})
        n_ok = sum(1 for s in scored if s.get("correct"))
        n_live = sum(1 for s in scored if "error" not in s)
        print(f"allam-2-7b {name}: {n_ok}/{n_live} correct", flush=True)
        results["variants"][name] = {"correct": n_ok, "live": n_live, "trials": scored}
        time.sleep(0.5)

    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, "w") as f:
        json.dump(results, f, indent=1)
    print("written " + out_path)


if __name__ == "__main__":
    main()
