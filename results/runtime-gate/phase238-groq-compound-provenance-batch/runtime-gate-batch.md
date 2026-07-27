# Runtime provider gate batch

- Name: `phase238-groq-compound-provenance-batch`
- Trials/calls: 5/5
- Provider successes: 0
- Execution failures: 5
- Durable reopens: 5
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 0/0
- Provider latency p50/p95/max: `7.573034003s` / `15.032659188s` / `15.032659188s`
- Selected bindings: `map[groq-compound:5]`
- Finish reasons: `map[:5]`
- Framing classes: `map[]`
- Second acquire reasons: `map[:5]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
