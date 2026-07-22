# Runtime provider gate batch

- Name: `phase-124-nim-json-batch`
- Trials/calls: 3/3
- Provider successes: 3
- Durable reopens: 3
- Expected matches: 3
- JSON valid: 3
- Tokens input/output: 288/33
- Provider latency p50/p95/max: `430.574872ms` / `931.47296ms` / `931.47296ms`
- Selected bindings: `map[nvidia-mistral-small-4:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[exact:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
