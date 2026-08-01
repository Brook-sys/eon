#!/usr/bin/env python3
"""Bounded live-fire sweep runner for massive-fire-sweep-v1.

Reads scripts/sweep/manifest.json + tasks.json, executes a deterministic
subset (sweep_slice parameter) of provider x model x task x temperature x rep
combinations, scores exact_lines responses against expectations, and writes
auditable JSON artifacts under results/sweeps/<name>/<slice>/.

Hard limits come from the manifest's limits block and are fail-closed.
All HTTP calls include a UA header (Cloudflare 1010 rejection otherwise,
confirmed 2026-08-01: probe without UA -> HTTP 403 on /chat/completions
even with valid API key; with UA -> HTTP 200 in <300ms).
"""
import json, os, sys, time, hashlib, urllib.request, urllib.error, argparse, statistics
from concurrent.futures import ThreadPoolExecutor

UA = "motor-autonomo-sweep/1.0 (python-urllib; bounded research sweep)"

def now_iso():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

def call_model(prov, model, prompt, temperature, max_tokens, timeout, budget, retries):
    """Single bounded chat.completions call with retry-after respect."""
    url = prov["base_url"].rstrip("/") + "/chat/completions"
    key = os.environ[prov["api_key_env"]]
    last_err = None
    for attempt in range(retries + 1):
        body = {"model": model, "messages": [{"role": "user", "content": prompt}],
                "temperature": temperature, "max_tokens": max_tokens}
        req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={
            "Authorization": "Bearer " + key, "Content-Type": "application/json",
            "User-Agent": UA})
        t0 = time.monotonic()
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                elapsed = (time.monotonic() - t0) * 1000
                budget[0] -= elapsed / 1000.0
                j = json.loads(r.read())
                usage = j.get("usage", {})
                txt = ""
                for c in j.get("choices", []):
                    txt += (c.get("message") or {}).get("content", "") or ""
                return {"ok": True, "latency_ms": round(elapsed, 1),
                        "finish_reason": (j.get("choices") or [{}])[0].get("finish_reason"),
                        "prompt_tokens": usage.get("prompt_tokens"),
                        "completion_tokens": usage.get("completion_tokens"),
                        "total_tokens": usage.get("total_tokens"),
                        "response_text": txt}
        except urllib.error.HTTPError as e:
            elapsed = (time.monotonic() - t0) * 1000
            budget[0] -= elapsed / 1000.0
            retry_after = None
            for h, v in e.headers.items():
                if h.lower() == "retry-after":
                    try: retry_after = int(v)
                    except ValueError: pass
            err_body = ""
            try: err_body = e.read()[:400].decode(errors="replace")
            except Exception: pass
            last_err = {"ok": False, "http_status": e.code, "latency_ms": round(elapsed, 1),
                        "retry_after": retry_after, "error_class": ("throttle" if e.code == 429 else
                            "auth" if e.code in (401, 403) else
                            "overload" if e.code in (529, 503) else
                            "server" if e.code >= 500 else "client"),
                        "error_body": err_body}
            if e.code == 429 and retry_after and attempt < retries and budget[0] >= retry_after:
                time.sleep(retry_after)
                continue
            break
        except Exception as e:
            elapsed = (time.monotonic() - t0) * 1000
            budget[0] -= elapsed / 1000.0
            last_err = {"ok": False, "latency_ms": round(elapsed, 1),
                        "error_class": "transport", "error_body": repr(e)[:300]}
            break
    return last_err

def parse_lines(text, keys):
    """Parse expected KEY: value lines; exact prefix match, strip whitespace."""
    out = {}
    for line in text.splitlines():
        for k in keys:
            if line.startswith(k + ":"):
                out[k] = line[len(k) + 1:].strip()
    return out

def score_task(task, response_text):
    """Exact-line scorer. Returns (all_correct, field_results)."""
    scoring = task.get("scoring", "")
    if not scoring.startswith("exact_lines:"):
        return False, {"error": "unknown scoring " + scoring}
    keys = scoring.split(":", 1)[1].split(",")
    expected = task["expected"]
    got = parse_lines(response_text, keys)
    fields = {}
    all_ok = True
    for k, exp in zip(keys, [expected.get(k.lower()) for k in keys]):
        g = got.get(k)
        ok = (g is not None and g == str(exp))
        fields[k] = {"expected": exp, "got": g, "ok": ok}
        if not ok:
            all_ok = False
    return all_ok, fields

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--slice-name", required=True)
    ap.add_argument("--models", default="", help="comma-separated provider/id:model filter; empty=all ok from probe")
    ap.add_argument("--temps", default="0.0")
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--tasks", default="")
    args = ap.parse_args()

    root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    manifest = json.load(open(os.path.join(root, "scripts/sweep/manifest.json")))
    tasks_doc = json.load(open(os.path.join(root, "scripts/sweep/tasks.json")))
    L = manifest["limits"]
    temps = [float(t) for t in args.temps.split(",")]
    task_list = tasks_doc["tasks"]
    if args.tasks:
        keep = set(args.tasks.split(","))
        task_list = [t for t in task_list if t["id"] in keep]

    # Model allowlist: probe-last.json ok models, or explicit filter
    probe_path = os.path.join(root, "scripts/sweep/probe-last.json")
    ok_models = []
    if os.path.exists(probe_path):
        probe = json.load(open(probe_path))
        ok_models = [(r["provider"], r["model"]) for r in probe["results"] if r["status"] == "ok"]
    if args.models:
        wanted = set()
        for m in args.models.split(","):
            prov, model = m.split(":", 1)
            wanted.add((prov, model))
        ok_models = [pm for pm in ok_models if pm in wanted]
    providers = {p["id"]: p for p in manifest["providers"]}
    missing = [prov for prov, _ in ok_models if prov not in providers]
    if missing:
        sys.exit("provider id not in manifest: " + repr(missing))

    jobs = []
    for prov_id, model in ok_models:
        for task in task_list:
            for t in temps:
                for rep in range(1, args.reps + 1):
                    if len(jobs) >= L["max_calls_total"]:
                        break
                    jobs.append((prov_id, model, task, t, rep))
    total = len(jobs)
    print(f"[sweep] planned calls={total} cap={L['max_calls_total']} per_model_cap={L['max_calls_per_model']}")

    budget = [600.0]  # seconds of wall clock consumed by calls
    results = []
    calls_ok = 0
    def run_one(job):
        nonlocal calls_ok
        prov_id, model, task, temp, rep = job
        r = call_model(providers[prov_id], model, task["prompt"], temp,
                       L["max_tokens_per_call"], L["timeout_per_call_seconds"],
                       budget, L["max_retries_per_call"])
        rec = {"provider": prov_id, "model": model, "task": task["id"], "temperature": temp,
               "rep": rep, "prompt_sha256": hashlib.sha256(task["prompt"].encode()).hexdigest()[:16],
               "outcome": "ok" if r.get("ok") else "error"}
        if r.get("ok"):
            correct, fields = score_task(task, r["response_text"])
            rec.update({"latency_ms": r["latency_ms"], "prompt_tokens": r["prompt_tokens"],
                        "completion_tokens": r["completion_tokens"], "total_tokens": r["total_tokens"],
                        "finish_reason": r["finish_reason"], "response_text": r["response_text"],
                        "scored_correct": correct, "field_results": fields})
            calls_ok += 1
        else:
            rec["error"] = {k: v for k, v in r.items() if k != "ok"}
        results.append(rec)
        status = "OK" if rec["outcome"] == "ok" else "ERR"
        extra = f" -> correct={rec['scored_correct']}" if status == "OK" else f" -> {rec['error'].get('error_class')}"
        print(f"[{status}] {prov_id}/{model} {task['id']} t={temp} r{rep}{extra}", flush=True)

    with ThreadPoolExecutor(max_workers=L["concurrency"]) as ex:
        list(ex.map(run_one, jobs))

    lat_ok = [r["latency_ms"] for r in results if r["outcome"] == "ok"]
    by_model = {}
    for r in results:
        k = f"{r['provider']}/{r['model']}"
        agg = by_model.setdefault(k, {"calls": 0, "ok": 0, "err": 0, "correct": 0})
        agg["calls"] += 1
        if r["outcome"] == "ok":
            agg["ok"] += 1
            agg["correct"] += int(r.get("scored_correct", False))
        else:
            agg["err"] += 1

    summary = {
        "sweep": manifest["name"],
        "slice": args.slice_name,
        "executed_at": now_iso(),
        "declared_limits": L,
        "calls_attempted": len(results),
        "calls_ok": calls_ok,
        "calls_error": len(results) - calls_ok,
        "latency_ms": {
            "p50": round(statistics.median(lat_ok), 1) if lat_ok else None,
            "p95": round(statistics.quantiles(lat_ok, n=20)[18], 1) if len(lat_ok) >= 20 else (max(lat_ok) if lat_ok else None),
            "max": max(lat_ok) if lat_ok else None,
        },
        "by_model": by_model,
    }
    out_dir = os.path.join(root, "results/sweeps", manifest["name"], args.slice_name)
    os.makedirs(out_dir, exist_ok=True)
    atomic_write(os.path.join(out_dir, "trials.json"),
                 json.dumps({"sweep": manifest["name"], "slice": args.slice_name,
                             "executed_at": now_iso(), "declared_limits": L,
                             "calls_attempted": len(results), "calls_ok": calls_ok,
                             "calls_error": len(results) - calls_ok, "trials": results}, indent=1))
    atomic_write(os.path.join(out_dir, "summary.json"), json.dumps(summary, indent=1))
    print(f"[sweep] wrote {out_dir}")
    print(json.dumps(summary, indent=1))

def atomic_write(path, data):
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        f.write(data)
        f.flush()
        os.fsync(f.fileno())
    os.link(tmp, path) if not os.path.exists(path) else None
    os.unlink(tmp)

if __name__ == "__main__":
    main()
