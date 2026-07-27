# Runtime provider gate batch

- Name: `phase282-groq-llama31-complete-evidence-pair-variance`
- Trials/calls: 5/5
- Provider successes: 5
- Execution failures: 0
- Durable reopens: 5
- Expected matches: 0
- JSON valid: 5
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 945/240
- Provider latency p50/p95/max: `261.502582ms` / `350.20757ms` / `350.20757ms`
- Selected bindings: `map[groq-llama31-8b-live:5]`
- Finish reasons: `map[stop:5]`
- Framing classes: `map[valid_json_mismatch:5]`
- Second acquire reasons: `map[resource_resource_rate_limit:5]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
