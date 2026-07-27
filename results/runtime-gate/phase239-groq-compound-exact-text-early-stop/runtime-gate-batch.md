# Runtime provider gate batch

- Name: `phase239-groq-compound-exact-text-early-stop`
- Trials/calls: 3/3
- Provider successes: 1
- Execution failures: 3
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 1090/154
- Provider latency p50/p95/max: `607.925854ms` / `1.0304433s` / `1.0304433s`
- Selected bindings: `map[groq-compound:3]`
- Finish reasons: `map[:2 stop:1]`
- Framing classes: `map[exact:1]`
- Second acquire reasons: `map[:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
