# Runtime provider gate batch

- Name: `phase226-groq-llama31-8b-provenance-correction`
- Trials/calls: 5/5
- Provider successes: 5
- Execution failures: 0
- Durable reopens: 5
- Expected matches: 0
- JSON valid: 5
- Schema evaluated/adherent/content-complete: 5/5/5
- Changes valid: 5
- Tokens input/output: 3490/740
- Provider latency p50/p95/max: `446.157232ms` / `563.132817ms` / `563.132817ms`
- Selected bindings: `map[groq-llama31-8b:5]`
- Finish reasons: `map[stop:5]`
- Framing classes: `map[valid_json_mismatch:5]`
- Second acquire reasons: `map[resource_resource_rate_limit:5]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
