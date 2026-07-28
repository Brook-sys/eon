# Runtime provider gate batch

- Name: `phase295-groq-llama31-report-crash-adoption`
- Trials/calls: 2/2
- Provider successes: 2
- Execution failures: 2
- Durable reopens: 2
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 442/108
- Provider latency p50/p95/max: `388.499955ms` / `393.327876ms` / `393.327876ms`
- Selected bindings: `map[groq-llama31-8b-live:2]`
- Finish reasons: `map[stop:2]`
- Framing classes: `map[markdown_fence:2]`
- Second acquire reasons: `map[:2]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
