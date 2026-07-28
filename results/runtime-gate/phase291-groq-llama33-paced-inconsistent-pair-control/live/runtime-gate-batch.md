# Runtime provider gate batch

- Name: `phase291-groq-llama33-paced-inconsistent-pair-control`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 663/129
- Provider latency p50/p95/max: `340.9656ms` / `362.926115ms` / `362.926115ms`
- Selected bindings: `map[groq-llama33-70b-live:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[valid_json_mismatch:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
