# Runtime provider gate batch

- Name: `phase305-groq-allam2-128-token-ceiling-control`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 954/249
- Provider latency p50/p95/max: `389.144527ms` / `461.571808ms` / `461.571808ms`
- Selected bindings: `map[groq-allam2:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[valid_json_mismatch:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
