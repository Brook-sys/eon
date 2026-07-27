# Runtime provider gate batch

- Name: `phase240-groq-llama33-70b-exact-text-batch`
- Trials/calls: 5/5
- Provider successes: 5
- Execution failures: 5
- Durable reopens: 5
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 470/10
- Provider latency p50/p95/max: `267.755658ms` / `322.614306ms` / `322.614306ms`
- Selected bindings: `map[groq-llama33-70b:5]`
- Finish reasons: `map[stop:5]`
- Framing classes: `map[exact:5]`
- Second acquire reasons: `map[:5]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
