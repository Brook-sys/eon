#!/usr/bin/env python3
"""Teste de fogo Phase 371 A: em que teto de max_tokens o qwen/qwen3.6-27b (Groq)
sai da fase <think> e produz o JSON pedido?

Hipotese (derivada do Phase 370, dashboard-v2 copy-quality): qwen3.6-27b consome
todo o budget em shadow reasoning a 220 tokens, nunca produzindo o JSON. Existe um
limiar de max_tokens (512-2048) em que a fase <think> encerra e o JSON aparece.

Cenario: mesmo contrato do Phase 370 (micro-copy PT-BR para overview-empty),
max_tokens em {512, 1024, 2048}, 5 trials por degrau, temperatura 0.9.

Limites declarados antes da execucao: 15 chamadas Groq no maximo, deadline 60 s/call,
interrupcao imediata em HTTP 429 persistente (2x seguidos) ou ausencia de ganho
apos o primeiro degrau com 5/5 validos.

Metricas: has_think_open, think_end_token_estimate (tokens antes de </think>),
json_valid, bytes uteis, P50/P95 latencia. Sem authority: resultado e evidencia.
"""
import json, os, sys, urllib.request, urllib.error, time, re

GROQ_KEY = os.environ.get("GROQ_API_KEY", "").strip()
if not GROQ_KEY:
    print("GROQ_API_KEY ausente", file=sys.stderr)
    sys.exit(2)

URL = "https://api.groq.com/openai/v1/chat/completions"
MODEL = "qwen/qwen3.6-27b"
LADDER = [512, 1024, 2048]
TRIALS = 5
TEMP = 0.9

PROMPT = (
    "Escreva micro-copy em portugues brasileiro para o estado vazio (empty state) da pagina "
    "'visao geral (overview) — painel de status do motor autonomo' do dashboard do motor-autonomo. "
    "Responda APENAS com um objeto JSON cru, sem markdown nem comentarios, com exatamente estas chaves: "
    '"title" (string curta, tom honesto e direto, sem emoji), '
    '"body" (uma frase curta explicando o que aparecera aqui quando houver dados), '
    '"action_hint" (string opcional curta ou null). '
    'Nao use TODO, lorem ipsum, nem texto placeholder.'
)


def call(max_tok, trial):
    body = json.dumps({
        "model": MODEL,
        "messages": [{"role": "user", "content": PROMPT}],
        "max_tokens": max_tok,
        "temperature": TEMP,
    }).encode()
    req = urllib.request.Request(URL, data=body, method="POST", headers={
        "Authorization": "Bearer " + GROQ_KEY,
        "Content-Type": "application/json",
        "User-Agent": "motor-autonomo-qwenladder/1.0 (python-urllib; thinking-budget research)",
    })
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            lat = time.monotonic() - t0
            payload = json.loads(r.read().decode())
            choice = payload["choices"][0]
            usage = payload.get("usage", {}) or {}
            details = usage.get("completion_tokens_details") or {}
            return {"ok": True, "latency_ms": round(lat * 1000),
                    "text": choice["message"].get("content") or "",
                    "finish": choice.get("finish_reason"),
                    "input_tokens": usage.get("prompt_tokens"),
                    "output_tokens": usage.get("completion_tokens"),
                    "reasoning_tokens": details.get("reasoning_tokens"),
                    "max_tokens": max_tok, "trial": trial,
                    "http_status": r.status}
    except urllib.error.HTTPError as e:
        err_body = ""
        try:
            err_body = e.read().decode()[:300]
        except Exception:
            pass
        return {"ok": False, "latency_ms": round((time.monotonic() - t0) * 1000),
                "http_status": e.code, "error": err_body,
                "retry_after": e.headers.get("Retry-After"),
                "max_tokens": max_tok, "trial": trial}
    except Exception as e:
        return {"ok": False, "latency_ms": round((time.monotonic() - t0) * 1000),
                "error": str(e)[:300], "max_tokens": max_tok, "trial": trial}


THINK_OPEN = re.compile(r"<think>", re.IGNORECASE)
THINK_CLOSE = re.compile(r"</think>", re.IGNORECASE)


def analyze(rec):
    if not rec["ok"]:
        return {"category": "http_error"}
    txt = rec["text"]
    an = {
        "has_think_open": bool(THINK_OPEN.search(txt)),
        "has_think_close": bool(THINK_CLOSE.search(txt)),
        "content_bytes": len(txt.encode("utf-8", "replace")),
    }
    m = THINK_CLOSE.search(txt)
    if m:
        an["json_region"] = txt[m.end():].strip()
    else:
        an["json_region"] = txt.strip()
    cand = an["json_region"]
    if cand.startswith("```"):
        parts = cand.split("```", 2)
        cand = parts[1] if len(parts) > 1 else cand
        cand = cand.lstrip("json\n ")
    try:
        obj = json.loads(cand)
        an["category"] = "valid_json"
        an["keys_ok"] = {"title", "body"} <= set(obj.keys())
        an["title_len"] = len(str(obj.get("title", "")))
    except json.JSONDecodeError:
        an["category"] = "no_valid_json"
        an["region_head"] = cand[:80]
    return an


def pct(vals, p):
    if not vals:
        return None
    s = sorted(vals)
    k = max(0, min(len(s) - 1, int((p / 100.0) * (len(s) - 1) + 0.5)))
    return s[k]


def main():
    out = {"started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
           "config": {"model": MODEL, "ladder": LADDER, "trials": TRIALS, "temp": TEMP},
           "steps": []}
    consec_429 = 0
    for max_tok in LADDER:
        step = {"max_tokens": max_tok, "trials": []}
        lats, valids, finishes = [], 0, {}
        think_open_ct = 0
        for t in range(TRIALS):
            rec = call(max_tok, t)
            rec["analysis"] = analyze(rec)
            step["trials"].append(rec)
            if not rec["ok"]:
                if rec.get("http_status") == 429:
                    consec_429 += 1
                    if consec_429 >= 2:
                        out["early_stop"] = f"429 persistente em max_tokens={max_tok} trial={t}"
                        step["early_stopped"] = True
                        _finalize(out, step, lats, valids, finishes, think_open_ct)
                        json_dump(out)
                        return
                continue
            consec_429 = 0
            lats.append(rec["latency_ms"])
            f = rec.get("finish") or "?"
            finishes[f] = finishes.get(f, 0) + 1
            if rec["analysis"]["category"] == "valid_json":
                valids += 1
            if rec["analysis"].get("has_think_open"):
                think_open_ct += 1
        _finalize(out, step, lats, valids, finishes, think_open_ct)
        out["steps"].append(step)
        # early stop se 5/5 validos: nao ha ganho em subir o degrau
        if valids == TRIALS:
            out["early_stop"] = (f"treshold encontrado em max_tokens={max_tok}: "
                                 f"{TRIALS}/{TRIALS} JSON valido; degraus superiores nao executados")
            json_dump(out)
            return
    json_dump(out)


def _finalize(out, step, lats, valids, finishes, think_open_ct):
    step["summary"] = {
        "valid_json": valids, "trials": len(step["trials"]),
        "p50_ms": pct(lats, 50), "p95_ms": pct(lats, 95),
        "finish_counts": finishes, "think_open_count": think_open_ct,
    }


def json_dump(out):
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                        "qwen-thinking-ladder-results.json")
    with open(path, "w") as fh:
        json.dump(out, fh, ensure_ascii=False, indent=1, default=list)
    print(f"[wrote] {path}")


if __name__ == "__main__":
    main()
