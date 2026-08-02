#!/usr/bin/env python3
"""Probe: does qwen3.6-27b on Groq suppress thinking with reasoning_effort=none?

Context: heartbeat-2026-08-01-multi-model-floor found qwen/qwen3.6-27b emits
raw thinking into content and truncates at max_tokens=256 (finish_reason=length,
0/6 correct). This probe tests whether Groq's reasoning_effort=none parameter
disables thinking so the model can produce parseable output within a bounded
token budget.

Run:
    set -a; source .provider-secrets.env; set +a
    python3 scripts/sweep/probe_qwen_thinking.py
"""
import json, os, sys, time, urllib.request, urllib.error

UA = "motor-autonomo-sweep/1.0 (python-urllib; bounded research sweep)"
URL = "https://api.groq.com/openai/v1/chat/completions"

EXTRACT_PROMPT = (
    "You are a deterministic extractor. Read the facts, obey constraints, and "
    "respond with EXACTLY two lines in this format.\n"
    "DATE: <iso-date>\nSOURCE: <id>\n\n"
    "Facts:\n"
    "- Source S-17 was published on 2025-11-03. The later review did not change the original publication date.\n"
    "- A separate source S-18 was published on 2025-11-04.\n\n"
    "Constraints:\n"
    "- Return the original publication date of S-17.\n"
    "- Output only the two lines, nothing else."
)

VARIANTS = [
    ("baseline_temp0_mt64", {"temperature": 0.0, "max_tokens": 64}),
    ("effort_none_mt64", {"temperature": 0.0, "max_tokens": 64, "reasoning_effort": "none"}),
    ("effort_none_mt256", {"temperature": 0.0, "max_tokens": 256, "reasoning_effort": "none"}),
]


def main():
    key = os.environ.get("GROQ_API_KEY")
    if not key:
        sys.exit("GROQ_API_KEY not set; source .provider-secrets.env first")
    results = []
    for name, extra in VARIANTS:
        body = {"model": "qwen/qwen3.6-27b",
                "messages": [{"role": "user", "content": EXTRACT_PROMPT}], **extra}
        req = urllib.request.Request(
            URL, data=json.dumps(body).encode(),
            headers={"Authorization": "Bearer " + key,
                     "Content-Type": "application/json", "User-Agent": UA})
        t0 = time.monotonic()
        rec = {"variant": name, "request": extra}
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                j = json.loads(r.read())
                u = j.get("usage", {})
                c = j["choices"][0]
                msg = c.get("message") or {}
                txt = msg.get("content") or ""
                rec.update({
                    "http_status": 200,
                    "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                    "finish_reason": c.get("finish_reason"),
                    "completion_tokens": u.get("completion_tokens"),
                    "has_reasoning_content": bool(msg.get("reasoning_content")),
                    "content": txt,
                    "correct": txt.strip() == "DATE: 2025-11-03\nSOURCE: S-17",
                })
                print("[%s] 200 %.0fms finish=%s tokens=%s reasoning_content=%s correct=%s",
                      name, rec["latency_ms"], rec["finish_reason"],
                      rec["completion_tokens"], rec["has_reasoning_content"], rec["correct"])
                print("  content:", repr(txt[:200]))
        except urllib.error.HTTPError as e:
            rec.update({"http_status": e.code,
                        "latency_ms": round((time.monotonic() - t0) * 1000, 1),
                        "error_body": e.read()[:400].decode(errors="replace")})
            print("[%s] HTTP %s: %s", name, e.code, rec["error_body"][:200])
        except Exception as e:
            rec.update({"error": repr(e)})
            print("[%s] EXC %r", name, e)
        results.append(rec)
    out = {"probed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
           "model": "qwen/qwen3.6-27b", "results": results}
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "probe_qwen_thinking_result.json")
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(out, f, indent=1)
    os.link(tmp, path) if not os.path.exists(path) else None
    os.remove(tmp)
    print("wrote", path)


if __name__ == "__main__":
    main()
