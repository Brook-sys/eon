# Runtime provider gate batch

- Name: `phase309-groq-qwen3-duplicate-evidence`
- Trials/calls: 2/2
- Provider successes: 0
- Execution failures: 2
- Durable reopens: 2
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 0/0
- Provider latency p50/p95/max: `35.609165ms` / `216.235552ms` / `216.235552ms`
- Selected bindings: `map[groq-qwen3:2]`
- Finish reasons: `map[:2]`
- Framing classes: `map[]`
- Second acquire reasons: `map[:2]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
