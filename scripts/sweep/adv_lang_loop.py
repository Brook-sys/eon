#!/usr/bin/env python3
"""Prompt improvement loop for adv-language-degradation.

Tested hypothesis:
  In Portuguese (PT-BR), models omit the required prefix `DATE:` / `SOURCE:`
  (emitting bare values like `2025-11-03\nS-17`), because the instruction
  `DATE: <iso-date>` in an otherwise Portuguese prompt is interpreted as an
  example/placeholder rather than an explicit mandatory prefix requirement.

Variations to test:
  v0 (baseline): Portuguese prompt as in tasks.json (0/5 on all models in baseline)
  v1 (explicit-format-pt): Adds explicit prefix instruction in Portuguese:
     "A primeira linha DEVE começar com 'DATE: ' e a segunda com 'SOURCE: '. Não omita os prefixos."
  v2 (restate-post-pt): Reinterprets the restatement pattern after constraints:
     "Lembre-se: responda EXATAMENTE em duas linhas, começando com os prefixos 'DATE: ' e 'SOURCE: '."
  v3 (fewshot-pt): Adds a mini 2-line example in PT-BR showing exact format.

Matrix: 4 Groq models x 3 new variants x 5 reps = 60 calls bounded.
"""
import os, sys, json, time, urllib.request

SECRETS_PATH = "/home/node/.openclaw/workspace/motor-autonomo/.provider-secrets.env"
if os.path.exists(SECRETS_PATH):
    with open(SECRETS_PATH) as f:
        for line in f:
            if line.strip() and not line.startswith('#'):
                k, v = line.strip().split('=', 1)
                v = v.strip("'")
                os.environ[k] = v

GROQ_KEY = os.environ.get("GROQ_API_KEY", "")
if not GROQ_KEY:
    print("GROQ_API_KEY missing")
    sys.exit(1)

MODELS = ["llama-3.1-8b-instant", "llama-3.3-70b-versatile", "allam-2-7b", "qwen/qwen3.6-27b"]

PROMPT_V1 = """Você é um extrator determinístico. Leia os fatos, obedeça as restrições e responda com EXATAMENTE duas linhas.

Fatos:
- Source S-17 foi publicada em 2025-11-03. A revisão posterior não mudou a data original.
- Uma fonte separada S-18 foi publicada em 2025-11-04.

Restrições:
- Retorne a data de publicação original de S-17.
- A primeira linha DEVE começar rigorosamente com 'DATE: ' seguido da data ISO.
- A segunda linha DEVE começar rigorosamente com 'SOURCE: ' seguido do ID da fonte.
- Não omita os prefixos 'DATE: ' e 'SOURCE: '. Responda apenas as duas linhas, nada mais."""

PROMPT_V2 = """Você é um extrator determinístico. Leia os fatos, obedeça as restrições e responda com EXATAMENTE duas linhas.
DATE: <iso-date>
SOURCE: <id>

Fatos:
- Source S-17 foi publicada em 2025-11-03. A revisão posterior não mudou a data original.
- Uma fonte separada S-18 foi publicada em 2025-11-04.

Restrições:
- Retorne a data de publicação original de S-17.
- Responda apenas as duas linhas, nada mais.

Nota importante: Toda linha DEVE obrigatoriamente começar com seu rótulo ('DATE: ' ou 'SOURCE: '). Nunca emita datas ou IDs nus sem o rótulo prefixado."""

PROMPT_V3 = """Você é um extrator determinístico. Leia os fatos, obedeça as restrições e responda com EXATAMENTE duas linhas no formato do exemplo.

Exemplo de formato esperado:
DATE: 2020-01-01
SOURCE: S-00

Fatos:
- Source S-17 foi publicada em 2025-11-03. A revisão posterior não mudou a data original.
- Uma fonte separada S-18 foi publicada em 2025-11-04.

Restrições:
- Retorne a data de publicação original de S-17.
- Responda apenas as duas linhas com prefixos DATE: e SOURCE:, nada mais."""

VARIANTS = {
    "v1-explicit-format-pt": PROMPT_V1,
    "v2-restate-post-pt": PROMPT_V2,
    "v3-fewshot-pt": PROMPT_V3,
}

EXPECTED_DATE = "2025-11-03"
EXPECTED_SOURCE = "S-17"

def score_output(text):
    lines = [l.strip() for l in text.strip().splitlines() if l.strip()]
    if len(lines) != 2:
        return False, f"line_count={len(lines)}"
    
    date_ok = lines[0] == f"DATE: {EXPECTED_DATE}"
    src_ok = lines[1] == f"SOURCE: {EXPECTED_SOURCE}"
    
    if date_ok and src_ok:
        return True, "exact_match"
    return False, f"line0='{lines[0]}' line1='{lines[1]}'"

def call_groq(model, prompt):
    url = "https://api.groq.com/openai/v1/chat/completions"
    payload = {
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "temperature": 0.0,
        "max_tokens": 64
    }
    req = urllib.request.Request(
        url,
        data=json.dumps(payload).encode('utf-8'),
        headers={
            "Authorization": f"Bearer {GROQ_KEY}",
            "Content-Type": "application/json", "User-Agent": "Go-http-client/1.1"
        }
    )
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read().decode('utf-8'))
            lat = (time.time() - t0) * 1000
            choice = data['choices'][0]
            text = choice['message'].get('content', '') or ''
            finish = choice.get('finish_reason', '')
            return True, lat, text, finish, ""
    except Exception as e:
        return False, (time.time() - t0)*1000, "", "", str(e)

results = {}
out_dir = "/home/node/.openclaw/workspace/motor-autonomo/results/sweeps/adv-lang-loop-2026-08-03"
os.makedirs(out_dir, exist_ok=True)

print("Starting adv-language-degradation prompt improvement loop...")
total_calls = 0

for vname, prompt in VARIANTS.items():
    results[vname] = {}
    for model in MODELS:
        correct_count = 0
        trials = []
        for rep in range(5):
            time.sleep(0.3)
            ok, lat, text, finish, err = call_groq(model, prompt)
            total_calls += 1
            if not ok:
                trials.append({"rep": rep, "ok": False, "error": err})
                continue
            is_correct, why = score_output(text)
            if is_correct:
                correct_count += 1
            trials.append({
                "rep": rep,
                "ok": True,
                "latency_ms": lat,
                "finish_reason": finish,
                "response_text": text,
                "correct": is_correct,
                "score_why": why
            })
        results[vname][model] = {"correct": correct_count, "of": 5, "trials": trials}
        print(f"[{vname}] {model}: {correct_count}/5 correct")

summary = {
    "campaign": "adv-lang-loop-2026-08-03",
    "total_calls": total_calls,
    "variants": {}
}
for vname in VARIANTS:
    summary["variants"][vname] = {
        m: {"correct": results[vname][m]["correct"], "of": 5} for m in MODELS
    }

with open(f"{out_dir}/summary.json", "w") as f:
    json.dump(summary, f, indent=2)

with open(f"{out_dir}/trials.json", "w") as f:
    json.dump(results, f, indent=2)

print(f"Done! {total_calls} calls executed. Summary written to {out_dir}/summary.json")
