#!/usr/bin/env python3
"""Teste de fogo: qual modelo Groq gera melhor micro-copy PT-BR para o dashboard-v2?

Hipotese: modelos maiores (llama-3.3-70b, qwen3.6-27b) produzem copy mais clara
e honesto para empty-states do que llama-3.1-8b-instant.

Cenario: simular usuario real pedindo titulo+subtexto para 3 empty-states do
dashboard-autonomo (overview, cortex, runs). Saida estrita em JSON.

Metricas: json_valid, lingua (PT-BR?), contagem de placeholder artifacts (TODO/lorem),
comprimento util (chars), P50/P95 latencia. Sem authority: resultado e evidencia.
"""
import json, os, sys, urllib.request, urllib.error, time, statistics

BASE = os.path.dirname(os.path.abspath(__file__))
GROQ_KEY = os.environ.get("GROQ_API_KEY", "").strip()
if not GROQ_KEY:
    print("GROQ_API_KEY ausente", file=sys.stderr)
    sys.exit(2)

URL = "https://api.groq.com/openai/v1/chat/completions"
MODELS = ["llama-3.1-8b-instant", "qwen/qwen3.6-27b", "llama-3.3-70b-versatile"]
PAGES = [
    ("overview-empty", "visao geral (overview) — painel de status do motor autonomo"),
    ("cortex-empty", "cortex — centro de operacoes cognitivas / runs recentes"),
    ("audit-empty", "auditoria — trilha de eventos e decisoes"),
]
TRIALS = 5
MAX_TOK = 220
TEMP = 0.9

PROMPT_TMPL = (
    "Escreva micro-copy em portugues brasileiro para o estado vazio (empty state) da pagina '{page}' do dashboard do motor-autonomo. "
    "Responda APENAS com um objeto JSON cru, sem markdown nem comentarios, com exatamente estas chaves: "
    '"title" (string curta, tom honesto e direto, sem emoji, sem promessas falsas), '
    '"body" (uma frase curta explicando o que aparecera aqui quando houver dados), '
    '"action_hint" (string opcional curta ou null). '
    'Nao use TODO, lorem ipsum, nem texto placeholder.'
)


def call(model, page, trial):
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": PROMPT_TMPL.format(page=page)}],
        "max_tokens": MAX_TOK,
        "temperature": TEMP,
    }).encode()
    req = urllib.request.Request(URL, data=body, method="POST", headers={
        "Authorization": "Bearer " + GROQ_KEY,
        "Content-Type": "application/json",
        # Cloudflare rejeita 1010 sem UA (documentado em results/sweeps/.../ANALYSIS.md)
        "User-Agent": "motor-autonomo-copyprobe/1.0 (python-urllib; ui-copy research)",
    })
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            lat = time.monotonic() - t0
            payload = json.loads(r.read().decode())
            choice = payload["choices"][0]
            txt = choice["message"]["content"]
            usage = payload.get("usage", {})
            return {"ok": True, "latencies_ms": round(lat * 1000), "text": txt,
                    "finish": choice.get("finish_reason"),
                    "input_tokens": usage.get("prompt_tokens"),
                    "output_tokens": usage.get("completion_tokens"),
                    "trial": trial, "model": model, "page": page}
    except urllib.error.HTTPError as e:
        lat = time.monotonic() - t0
        err_body = ""
        try:
            err_body = e.read().decode()[:300]
        except Exception:
            pass
        return {"ok": False, "latencies_ms": round(lat * 1000), "http_status": e.code,
                "error": err_body, "retry_after": e.headers.get("Retry-After"),
                "trial": trial, "model": model, "page": page}
    except Exception as e:
        return {"ok": False, "latencies_ms": round((time.monotonic() - t0) * 1000),
                "error": str(e)[:300], "trial": trial, "model": model, "page": page}


def analyze(rec):
    if not rec["ok"]:
        return {"json_valid": False, "category": "http_error"}
    txt = rec["text"].strip()
    # extract raw JSON (tolerate code fences)
    candidate = txt
    if candidate.startswith("```"):
        parts = candidate.split("```", 2)
        candidate = parts[1] if len(parts) > 1 else candidate
        candidate = candidate.lstrip("json\n ")
    try:
        obj = json.loads(candidate)
        rec["parsed"] = obj
        placeholder = any(w in txt.lower() for w in ("todo", "lorem", "placeholder", "xxx"))
        title_len = len(str(obj.get("title", "")))
        return {"json_valid": True, "category": "valid", "placeholder": placeholder,
                "title_len": title_len,
                "keys_ok": set(("title", "body", "action_hint")) & set(obj.keys()) == {"title", "body"}}
    except json.JSONDecodeError:
        return {"json_valid": False, "category": "invalid_json", "raw_head": txt[:120]}


def pct(vals, p):
    if not vals:
        return None
    s = sorted(vals)
    k = max(0, min(len(s) - 1, int((p / 100.0) * (len(s) - 1) + 0.5)))
    return s[k]


def main():
    out = {"started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
           "config": {"models": MODELS, "trials": TRIALS, "max_tokens": MAX_TOK, "temp": TEMP,
                      "pages": [p for p, _ in PAGES]},
           "results": []}
    for model in MODELS:
        lats, valids, invalids, placeholders = [], 0, 0, 0
        per_model = {"model": model, "calls": []}
        for page, pdesc in PAGES:
            page_rows = []
            for t in range(TRIALS):
                rec = call(model, pdesc, t)
                rec["page_id"] = page
                an = analyze(rec)
                rec["analysis"] = an
                per_model["calls"].append(rec)
                page_rows.append({"trial": t, "ok": rec["ok"],
                                  "latency_ms": rec["latencies_ms"],
                                  "json_valid": an["json_valid"],
                                  "in": rec.get("input_tokens"), "out": rec.get("output_tokens")})
                if rec["ok"]:
                    lats.append(rec["latencies_ms"])
                    if an["json_valid"]:
                        valids += 1
                        if an.get("placeholder"):
                            placeholders += 1
                    else:
                        invalids += 1
                time.sleep(0.2)
            print(f"[{model}] {page}: " + " ".join(
                f"{'OK' if r['json_valid'] else 'BAD'}({r['latency_ms']}ms)" for r in page_rows))
        per_model["stats"] = {
            "calls_ok": valids + invalids,
            "json_valid": valids, "json_invalid": invalids,
            "placeholder_hits": placeholders,
            "p50_ms": pct(lats, 50), "p95_ms": pct(lats, 95),
            "min_ms": min(lats) if lats else None, "max_ms": max(lats) if lats else None,
        }
        out["results"].append(per_model)
    out["completed_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    dest = os.path.join(BASE, "copy-quality-results.json")
    with open(dest, "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=2)
    print("\nWROTE", dest)
    for r in out["results"]:
        print(r["model"], "->", json.dumps(r["stats"]))


if __name__ == "__main__":
    main()
