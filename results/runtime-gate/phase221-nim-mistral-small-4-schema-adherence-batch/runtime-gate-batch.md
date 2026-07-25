# Runtime provider gate batch

- Name: `phase221-nim-mistral-small-4-schema-adherence-batch`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 3
- Schema evaluated/adherent: 3/3
- Changes valid: 3
- Tokens input/output: 1842/458
- Provider latency p50/p95/max: `3.011110959s` / `3.152083717s` / `3.152083717s`
- Selected bindings: `map[nim-mistral-small-4:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[valid_json_mismatch:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
