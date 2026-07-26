# Runtime provider gate batch

- Name: `phase232-groq-gpt-oss-20b-provenance`
- Trials/calls: 3/3
- Provider successes: 0
- Execution failures: 3
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 0/0
- Provider latency p50/p95/max: `689.498185ms` / `783.801908ms` / `783.801908ms`
- Selected bindings: `map[groq-gpt-oss-20b:3]`
- Finish reasons: `map[:3]`
- Framing classes: `map[]`
- Second acquire reasons: `map[:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
