# Runtime provider gate batch

- Name: `phase231-groq-compound-mini-provenance`
- Trials/calls: 5/5
- Provider successes: 4
- Execution failures: 1
- Durable reopens: 5
- Expected matches: 0
- JSON valid: 4
- Schema evaluated/adherent/content-complete: 4/4/4
- Changes valid: 4
- Tokens input/output: 8230/2438
- Provider latency p50/p95/max: `2.860000314s` / `3.131355057s` / `3.131355057s`
- Selected bindings: `map[groq-compound-mini:5]`
- Finish reasons: `map[:1 stop:4]`
- Framing classes: `map[valid_json_mismatch:4]`
- Second acquire reasons: `map[:1 resource_resource_rate_limit:4]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
