# Runtime provider gate batch

- Name: `phase-124-groq-json-trial`
- Trials/calls: 3/3
- Provider successes: 3
- Durable reopens: 3
- Expected matches: 3
- JSON valid: 3
- Tokens input/output: 345/30
- Provider latency p50/p95/max: `279.646531ms` / `301.901075ms` / `301.901075ms`
- Selected bindings: `map[groq-llama-3.1-8b:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[exact:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
