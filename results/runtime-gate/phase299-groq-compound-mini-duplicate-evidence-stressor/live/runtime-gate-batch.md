# Runtime provider gate batch

- Name: `phase299-groq-compound-mini-duplicate-evidence-stressor`
- Trials/calls: 3/3
- Provider successes: 2
- Execution failures: 1
- Durable reopens: 3
- Expected matches: 2
- JSON valid: 2
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 1742/468
- Provider latency p50/p95/max: `954.555821ms` / `1.186689346s` / `1.186689346s`
- Selected bindings: `map[groq-compound-mini:3]`
- Finish reasons: `map[:1 stop:2]`
- Framing classes: `map[exact:2]`
- Second acquire reasons: `map[:1 resource_resource_rate_limit:2]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
