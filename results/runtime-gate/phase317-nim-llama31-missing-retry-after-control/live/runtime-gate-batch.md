# Runtime provider gate batch

- Name: `phase317-nim-llama31-missing-retry-after-control`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 2
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 594/107
- Provider latency p50/p95/max: `11.029754672s` / `11.459764071s` / `11.459764071s`
- Selected bindings: `map[nim-llama31-70b:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[exact:2 valid_json_mismatch:1]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
