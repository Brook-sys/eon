# Runtime provider gate batch

- Name: `phase308-nim-llama31-70b-duplicate-variance`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 3
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 477/93
- Provider latency p50/p95/max: `13.812893916s` / `27.81352062s` / `27.81352062s`
- Selected bindings: `map[nim-llama31-70b:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[exact:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
