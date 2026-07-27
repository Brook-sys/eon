# Runtime provider gate batch

- Name: `phase266-groq-llama31-8b-nested-json-variance`
- Trials/calls: 5/5
- Provider successes: 5
- Execution failures: 0
- Durable reopens: 5
- Expected matches: 5
- JSON valid: 5
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 585/100
- Provider latency p50/p95/max: `280.882453ms` / `301.835552ms` / `301.835552ms`
- Selected bindings: `map[groq-llama31-8b-instant:5]`
- Finish reasons: `map[stop:5]`
- Framing classes: `map[exact:5]`
- Second acquire reasons: `map[resource_resource_rate_limit:5]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
