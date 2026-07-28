# Runtime provider gate batch

- Name: `phase290-groq-compound-mini-paced-inconsistent-pair-control`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 3
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 2427/697
- Provider latency p50/p95/max: `1.000170057s` / `1.292753999s` / `1.292753999s`
- Selected bindings: `map[groq-compound-mini-live:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[exact:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
