"""Probe newly added NIM models from manifest.json against the live endpoint.

Scope: 2026-08-02 capability check for the four models added to the sweep
manifest (`meta/llama-3.1-70b-instruct`, `nvidia/llama-3.3-nemotron-super-49b-v1`,
`deepseek-ai/deepseek-v4-pro`, `mistralai/mistral-nemotron`). Each candidate gets
one minimal `READY` trial (max_tokens=8, temperature=0.0, 60s timeout) plus one
retry on HTTP 429/5xx, bounded to at most 8 live calls total.

Reads credentials from the environment (`NVIDIA_NIM_API_KEY`). Never prints or
persists secrets.
"""
import json, os, sys, time, urllib.request, urllib.error

CANDIDATES = [
    "meta/llama-3.1-70b-instruct",
    "nvidia/llama-3.3-nemotron-super-49b-v1",
    "deepseek-ai/deepseek-v4-pro",
    "mistralai/mistral-nemotron",
]
BASE_URL = "https://integrate.api.nvidia.com/v1"
OUT_PATH = "results/model-inventory/2026-08-02/nim-new-models-probe.json"
MAX_ATTEMPTS_PER_MODEL = 2
RETRYABLE_HTTP = {429, 500, 502, 503, 504}

key = os.environ.get("NVIDIA_NIM_API_KEY", "")
if not key:
    print("FATAL: missing env NVIDIA_NIM_API_KEY", file=sys.stderr)
    sys.exit(2)

def attempt(model):
    body = {"model": model, "messages": [{"role": "user", "content": "Reply with exactly: READY"}],
            "temperature": 0.0, "max_tokens": 8}
    url = BASE_URL.rstrip("/") + "/chat/completions"
    req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={
        "Authorization": "Bearer " + key, "Content-Type": "application/json",
        "User-Agent": "motor-autonomo-sweep/1.0 (python-urllib; capability probe)",
    })
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            j = json.loads(r.read())
            ch = (j.get("choices") or [{}])[0]
            txt = ch.get("message", {}).get("content", "").strip()[:40]
            usage = j.get("usage") or {}
            return {"http": 200, "latency_ms": round((time.monotonic() - t0) * 1000),
                    "content": txt, "finish_reason": ch.get("finish_reason"),
                    "prompt_tokens": usage.get("prompt_tokens"),
                    "completion_tokens": usage.get("completion_tokens"),
                    "status": "ok" if txt.startswith("READY") else "unexpected_content"}
    except urllib.error.HTTPError as e:
        body_err = e.read()[:300].decode(errors="replace")
        retry_after = e.headers.get("Retry-After") if e.headers else None
        return {"http": e.code, "latency_ms": round((time.monotonic() - t0) * 1000),
                "status": f"http_{e.code}", "error": body_err[:200],
                "retry_after": retry_after}
    except Exception as e:
        return {"http": None, "latency_ms": round((time.monotonic() - t0) * 1000),
                "status": "transport_error", "error": str(e)[:200]}

results = []
total_calls = 0
for model in CANDIDATES:
    attempts = []
    for n in range(1, MAX_ATTEMPTS_PER_MODEL + 1):
        total_calls += 1
        att = attempt(model)
        att["attempt"] = n
        attempts.append(att)
        print(f"{model} attempt {n}: {att['status']} http={att['http']} {att['latency_ms']}ms "
              f"content={att.get('content')!r} retry_after={att.get('retry_after')}")
        if att["status"] == "ok" or (att.get("http") or 0) not in RETRYABLE_HTTP:
            break
        time.sleep(2.0)
    final = attempts[-1]
    results.append({"model": model, "attempts": attempts, "final_status": final["status"]})

out = {"probed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
       "base_url": BASE_URL, "task": "exact_text_READY", "temperature": 0.0, "max_tokens": 8,
       "total_calls": total_calls, "candidates": CANDIDATES, "results": results}
os.makedirs(os.path.dirname(OUT_PATH), exist_ok=True)
open(OUT_PATH, "w").write(json.dumps(out, indent=1) + "\n")

ok = [r for r in results if r["final_status"] == "ok"]
print(f"\n{len(ok)}/{len(results)} new NIM models callable ({total_calls} calls) -> {OUT_PATH}")
for r in results:
    print(f"  {r['model']}: {r['final_status']}")
