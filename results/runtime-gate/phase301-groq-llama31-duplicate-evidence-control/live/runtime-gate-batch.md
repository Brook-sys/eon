# Runtime provider gate batch

- Name: `phase301-groq-llama31-duplicate-evidence-control`
- Trials/calls: 3/3
- Provider successes: 3
- Execution failures: 3
- Durable reopens: 3
- Expected matches: 0
- JSON valid: 0
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 753/147
- Provider latency p50/p95/max: `331.03797ms` / `381.297857ms` / `381.297857ms`
- Selected bindings: `map[groq-llama31:3]`
- Finish reasons: `map[stop:3]`
- Framing classes: `map[markdown_fence:3]`
- Second acquire reasons: `map[:3]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
