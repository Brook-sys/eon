# Runtime provider gate batch

- Name: `phase316-nim-deepseek-v4-flash-missing-retry-after`
- Trials/calls: 3/3
- Provider successes: 1
- Execution failures: 2
- Durable reopens: 3
- Expected matches: 1
- JSON valid: 1
- Schema evaluated/adherent/content-complete: 0/0/0
- Changes valid: 0
- Tokens input/output: 194/196
- Provider latency p50/p95/max: `991.793943ms` / `7.881349693s` / `7.881349693s`
- Selected bindings: `map[nim-deepseek-v4-flash:3]`
- Finish reasons: `map[:2 stop:1]`
- Framing classes: `map[exact:1]`
- Second acquire reasons: `map[:2 resource_resource_rate_limit:1]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
