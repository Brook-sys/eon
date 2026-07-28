# Runtime provider gate batch

- Name: `phase300-groq-compound-mini-paced-duplicate-evidence-control`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 3
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 2613/614
- Provider latency p50/p95/max: `1.015736741s` / `1.241183329s` / `1.241183329s`
- Selected bindings: `map[groq-compound-mini:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[exact:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
