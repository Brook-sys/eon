# Runtime provider gate batch

- Name: `phase223-nim-mistral-small-4-adversarial-types`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 3
- Schema evaluated/adherent: 3/3
- Changes valid: 3
- Tokens input/output: 1953/457
- Provider latency p50/p95/max: `2.813863145s` / `3.277954511s` / `3.277954511s`
- Selected bindings: `map[nim-mistral-small-4:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[valid_json_mismatch:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
