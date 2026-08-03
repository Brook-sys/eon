#!/usr/bin/env python3
"""Teste de fogo Phase 371 B: qual variacao de prompt elimina o markdown fence
em llama-3.1-8b-instant (Groq) — correcao real ou placebo?

Hipotese (Phase 370): 12/15 respostas do 8b-instant envolveram o JSON pedido em
fence ```json, apesar de instrucao explicita. Variacoes de prompt (negation
restatement, few-shot, output-echo contract, mesma instrucao com enfase no
primeiro byte) podem reduzir a taxa de fence abaixo de 50%.

Cenario: mesmo contrato do Phase 370 (micro-copy PT-BR para overview-empty),
max_tokens=220, temp 0.9, 5 trials por variacao:
  V0 = baseline (mesmo prompt da fase 370; controle placebo)
  V1 = negation-restatement (repete a restricao no fim do prompt)
  V2 = prefilled assistant (assistant comeca com '{' forcando continuacao)
  V3 = one-shot bogus (exemplo assumido de resposta ruim/erro) — few-shot errado

Limites: 20 chamadas Groq no maximo, 60 s/call, 2x 429 seguidos interrompem.

Metricas: json_valid (apos decode estrito raw), needs_fence_strip, previsibilidade.
Sem authority: resultado e evidencia para revisao do binding no dashboard-v2.
"""
import json, os, sys, urllib.request, urllib.error, time

GROQ_KEY = os.environ.get("GROQ_API_KEY", "").strip()
if not GROQ_KEY:
    print("GROQ_API_KEY ausente", file=sys.stderr)
    sys.exit(2)

URL = "https://api.groq.com/openai/v1/chat/completions"
MODEL = "llama-3.1-8b-instant"
MAX_TOK = 220
TEMP = 0.9
TRIALS = 5

PAGE_DESC = "visao geral (overview) — painel de status do motor autonomo"

BASE_BODY = (
    "Escreva micro-copy em portugues brasileiro para o estado vazio (empty state) da pagina "
    f"'{PAGE_DESC}' do dashboard do motor-autonomo. "
    "Responda APENAS com um objeto JSON cru, sem markdown nem comentarios, com exatamente estas chaves: "
    '"title" (string curta, tom honesto e direto, sem emoji), '
    '"body" (uma frase curta explicando o que aparecera aqui quando houver dados), '
    '"action_hint" (string opcional curta ou null). '
    "Nao use TODO, lorem ipsum, nem texto placeholder."
)

VARIANTS = {
    "v0-baseline": [
        {"role": "user", "content": BASE_BODY}
    ],
    "v1-negation-restatement": [
        {"role": "user", "content": BASE_BODY + (
            "\n\nIMPORTANTE: NAO escreva ```json, NAO use markdown, NAO use comentarios. "
            "O PRIMEIRO CARACTERE da sua resposta DEVE ser `{` e o ULTIMO DEVE ser `}`.")}
    ],
    "v2-prefilled-assistant": [
        {"role": "user", "content": BASE_BODY},
        {"role": "assistant", "content": "{"}
    ],
    "v3-few-shot-poison": [
        {"role": "user", "content": BASE_BODY},
        {"role": "assistant", "content": "```json\n{\"title\": \"EXEMPLO RUIM\", \"body\": \"nao copie\", \"action_hint\": null}\n```"},
        {"role": "user", "content": "A resposta acima foi rejeitada porque usou fence. "
                                    "Reescreva a sua propria micro-copy original para a pagina, sem fence, sem markdown, apenas JSON cru."}
    ],
}


def call(name, messages, trial):
    body = json.dumps({
        "model": MODEL, "messages": messages,
        "max_tokens": MAX_TOK, "temperature": TEMP,
    }).encode()
    req = urllib.request.Request(URL, data=body, method="POST", headers={
        "Authorization": "Bearer " + GROQ_KEY,
        "Content-Type": "application/json",
        "User-Agent": "motor-autonomo-promptvariations/1.0 (python-urllib; fence research)",
    })
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            lat = time.monotonic() - t0
            payload = json.loads(r.read().decode())
            choice = payload["choices"][0]
            txt = choice["message"].get("content") or ""
            # v2 prefilled: provider pode retornar somente o resto após '{'
            return {"ok": True, "latency_ms": round(lat * 1000), "variant": name,
                    "trial": trial, "text": txt, "finish": choice.get("finish_reason"),
                    "output_tokens": payload.get("usage", {}).get("completion_tokens")}
    except urllib.error.HTTPError as e:
        try:
            err = e.read().decode()[:200]
        except Exception:
            err = ""
        return {"ok": False, "latency_ms": round((time.monotonic() - t0) * 1000),
                "variant": name, "trial": trial, "http_status": e.code,
                "error": err, "retry_after": e.headers.get("Retry-After")}
    except Exception as e:
        return {"ok": False, "latency_ms": round((time.monotonic() - t0) * 1000),
                "variant": name, "trial": trial, "error": str(e)[:200]}


def analyze(rec):
    if not rec["ok"]:
        return {"category": "http_error"}
    raw = rec["text"]
    txt = raw.strip()
    needs_strip = txt.startswith("```") or "\n```" in txt
    candidate = txt
    if needs_strip:
        parts = candidate.split("```", 2)
        candidate = parts[1] if len(parts) > 1 else candidate
        candidate = candidate.lstrip("json\n ")
    # v2: se começamos com '{', o provider pode ter continuado sem refazer o char
    if not candidate.startswith("{") and rec["variant"] == "v2-prefilled-assistant":
        candidate = "{" + candidate
    try:
        obj = json.loads(candidate)
        return {"category": "valid_json", "needs_fence_strip": bool(needs_strip),
                "keys_ok": {"title", "body"} <= set(obj.keys()),
                "title_len": len(str(obj.get("title", "")))}
    except json.JSONDecodeError:
        return {"category": "no_valid_json", "needs_fence_strip": bool(needs_strip),
                "head": candidate[:90]}


def pct(vals, p):
    if not vals:
        return None
    s = sorted(vals)
    k = max(0, min(len(s) - 1, int((p / 100.0) * (len(s) - 1) + 0.5)))
    return s[k]


def main():
    out = {"started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
           "config": {"model": MODEL, "max_tokens": MAX_TOK, "temp": TEMP, "trials": TRIALS,
                      "variants": list(VARIANTS)},
           "variants": []}
    consec_429 = 0
    for name, messages in VARIANTS.items():
        block = {"variant": name, "trials": []}
        lats, valids, strips = [], 0, 0
        for t in range(TRIALS):
            rec = call(name, messages, t)
            rec["analysis"] = analyze(rec)
            block["trials"].append(rec)
            if not rec["ok"]:
                if rec.get("http_status") == 429:
                    consec_429 += 1
                    if consec_429 >= 2:
                        out["early_stop"] = f"429 persistente em variante={name} trial={t}"
                        block["early_stopped"] = True
                        break
                continue
            consec_429 = 0
            lats.append(rec["latency_ms"])
            if rec["analysis"]["category"] == "valid_json":
                valids += 1
            if rec["analysis"].get("needs_fence_strip"):
                strips += 1
        block["summary"] = {
            "valid": valids, "trials": len(block["trials"]),
            "fence_strip_needed": strips,
            "p50_ms": pct(lats, 50), "p95_ms": pct(lats, 95),
        }
        out["variants"].append(block)
        if out.get("early_stop"):
            break
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                        "prompt-variations-results.json")
    with open(path, "w") as fh:
        json.dump(out, fh, ensure_ascii=False, indent=1, default=list)
    print(f"[wrote] {path}")


if __name__ == "__main__":
    main()
