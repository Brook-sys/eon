# Runtime provider gate batch

- Name: `phase251-groq-gpt-oss-120b-proposed-changeset`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 3
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 2037/1152
- Provider latency p50/p95/max: `1.100097616s` / `1.189776866s` / `1.189776866s`
- Selected bindings: `map[groq-gpt-oss-120b:3]`
- Finish reasons: `map[length:3]`
- Framing classes: `map[invalid_json:3]`
- Second acquire reasons: `map[:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
