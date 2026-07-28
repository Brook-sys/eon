# Runtime provider gate batch

- Name: `phase313-groq-gptoss-duplicate-control`
- Trials/calls: 3/3
- Provider successes: 0
- Execution failures: 3
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 0/0
- Provider latency p50/p95/max: `431.335465ms` / `686.914533ms` / `686.914533ms`
- Selected bindings: `map[groq-gptoss20b:3]`
- Finish reasons: `map[:3]`
- Framing classes: `map[]`
- Second acquire reasons: `map[:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
