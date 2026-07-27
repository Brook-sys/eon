# Runtime provider gate batch

- Name: `phase241-groq-llama33-70b-exact-text-authority-free`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 3
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 282/6
- Provider latency p50/p95/max: `272.427125ms` / `432.370545ms` / `432.370545ms`
- Selected bindings: `map[groq-llama33-70b:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
