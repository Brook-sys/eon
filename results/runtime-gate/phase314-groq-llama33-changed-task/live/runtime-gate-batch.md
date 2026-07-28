# Runtime provider gate batch

- Name: `phase314-groq-llama33-changed-task`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 549/96
- Provider latency p50/p95/max: `376.13529ms` / `585.365448ms` / `585.365448ms`
- Selected bindings: `map[groq-llama33:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[valid_json_mismatch:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
