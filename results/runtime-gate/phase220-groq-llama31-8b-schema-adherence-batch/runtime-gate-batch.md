# Runtime provider gate batch

- Name: `phase220-groq-llama31-8b-schema-adherence-batch`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 3
- Schema evaluated/adherent: 3/3
- Changes valid: 3
- Tokens input/output: 1857/399
- Provider latency p50/p95/max: `515.555793ms` / `527.452407ms` / `527.452407ms`
- Selected bindings: `map[groq-llama31-8b:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[valid_json_mismatch:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
