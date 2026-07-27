# Runtime provider gate batch

- Name: `phase243-nim-mistral-small-4-nested-json-whitespace`
- Trials/calls: 2/2
- Provider successes: 0
- Execution failures: 2
- Durable reopens: 2
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 0/0
- Provider latency p50/p95/max: `121.650541ms` / `382.115309ms` / `382.115309ms`
- Selected bindings: `map[nim-mistral-small-4:2]`
- Finish reasons: `map[:2]`
- Framing classes: `map[]`
- Second acquire reasons: `map[:2]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
