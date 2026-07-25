# Runtime provider gate batch

- Name: `phase218a-nim-llama31-70b-schema-adherence`
- Trials/calls: 2/2
- Provider successes: 2
- Execution failures: 0
- Durable reopens: 2
- Expected matches: 0
- JSON valid: 2
- Tokens input/output: 1198/324
- Provider latency p50/p95/max: `8.85330722s` / `17.089986987s` / `17.089986987s`
- Selected bindings: `map[nim-meta-llama31-70b:2]`
- Finish reasons: `map[stop:2]`
- Framing classes: `map[valid_json_mismatch:2]`
- Second acquire reasons: `map[resource_resource_rate_limit:2]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
