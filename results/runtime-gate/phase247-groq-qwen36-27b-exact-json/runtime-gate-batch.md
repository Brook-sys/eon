# Runtime provider gate batch

- Name: `phase247-groq-qwen36-27b-exact-json`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 3
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 294/96
- Provider latency p50/p95/max: `342.442975ms` / `638.828595ms` / `638.828595ms`
- Selected bindings: `map[groq-qwen36-27b:3]`
- Finish reasons: `map[length:3]`
- Framing classes: `map[invalid_json:3]`
- Second acquire reasons: `map[:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
