# Runtime provider gate batch

- Name: `phase272-groq-llama31-8b-semantic-structural-replay`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 462/123
- Provider latency p50/p95/max: `302.897381ms` / `359.773397ms` / `359.773397ms`
- Selected bindings: `map[groq-llama31-8b-instant:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[valid_json_mismatch:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
