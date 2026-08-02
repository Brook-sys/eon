import json, os, sys, time, urllib.request, urllib.error

manifest = json.loads(open("scripts/sweep/manifest.json").read())
results = []
blocked_providers = set()

for prov in manifest["providers"]:
    key = os.environ.get(prov["api_key_env"], "")
    if not key:
        print(f"FATAL: missing env {prov['api_key_env']}", file=sys.stderr)
        sys.exit(2)
    if prov["id"] in blocked_providers:
        for m in prov["models"]:
            results.append({"provider": prov["id"], "model": m, "status": "skipped_geo_blocked"})
        continue
    for m in prov["models"]:
        body = {"model": m, "messages": [{"role": "user", "content": "Reply with exactly: READY"}], "temperature": 0.0, "max_tokens": 8}
        url = prov["base_url"].rstrip("/") + "/chat/completions"
        req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={
            "Authorization": "Bearer " + key, "Content-Type": "application/json",
            "User-Agent": "motor-autonomo-sweep/1.0 (python-urllib; research probe)"
        })
        t0 = time.monotonic()
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                j = json.loads(r.read())
                txt = (j.get("choices") or [{}])[0].get("message", {}).get("content", "").strip()[:40]
                lat = round((time.monotonic() - t0) * 1000)
                print(f"OK   {prov['id']}/{m}  {lat}ms  {txt!r}")
                results.append({"provider": prov["id"], "model": m, "status": "ok", "latency_ms": lat, "sample": txt})
        except urllib.error.HTTPError as e:
            body_err = e.read()[:300].decode(errors="replace")
            lat = round((time.monotonic() - t0) * 1000)
            if e.code == 403 and "unsupported_country" in body_err:
                print(f"GEO  {prov['id']}/{m}  HTTP 403 (country blocked)")
                results.append({"provider": prov["id"], "model": m, "status": "geo_blocked_403", "latency_ms": lat})
                blocked_providers.add(prov["id"])
            elif e.code == 404:
                print(f"DEPR {prov['id']}/{m}  HTTP 404 (not provisioned)")
                results.append({"provider": prov["id"], "model": m, "status": "deprecated_404", "latency_ms": lat})
            elif e.code == 410:
                print(f"DEPR {prov['id']}/{m}  HTTP 410 (sunsetted)")
                results.append({"provider": prov["id"], "model": m, "status": "sunsetted_410", "latency_ms": lat})
            else:
                print(f"ERR  {prov['id']}/{m}  HTTP {e.code} {body_err[:80]}")
                results.append({"provider": prov["id"], "model": m, "status": f"http_{e.code}", "latency_ms": lat, "error": body_err[:200]})
        except Exception as e:
            lat = round((time.monotonic() - t0) * 1000)
            print(f"TIME {prov['id']}/{m}  timeout/transport {str(e)[:60]}")
            results.append({"provider": prov["id"], "model": m, "status": "timeout", "latency_ms": lat, "error": str(e)[:200]})

ok = [r for r in results if r["status"] == "ok"]
out = {"probed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
       "total": len(results), "ok": len(ok), "results": results}
open("scripts/sweep/probe-last.json", "w").write(json.dumps(out, indent=1))
print(f"\nSummary: {len(ok)}/{len(results)} models available")
for r in results:
    print(f"  {r['provider']}/{r['model']}: {r['status']}")
sys.exit(0 if len(ok) >= 1 else 1)
