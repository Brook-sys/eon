# Runtime provider gate batch

- Name: `phase285-nim-llama31-70b-complete-evidence-pair-variance`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 0
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 3
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 543/105
- Provider latency p50/p95/max: `2.069082101s` / `2.41261725s` / `2.41261725s`
- Selected bindings: `map[nim-llama31-70b-live:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[valid_json_mismatch:3]`
- Second acquire reasons: `map[resource_resource_rate_limit:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
