# Runtime provider gate campaign

- Name: `nim-primary-circuit-to-groq-minimal-exact-json-2026-07-22-1340`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-primary`
- Selected route: `groq` / `groq-fallback`
- Provider success: `true`
- Provider latency: `313.539273ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 50
- Response SHA-256: `7d97f9b3b9fb23e106ad9455454b813cb4b6665dba757fbd5398521f0ae8533e`
- Expected response configured: `true`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T16:45:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-fallback | 0 | 1 | 1 | 123 | 0 |  |
| model-binding:nim-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-22T16:45:35Z |
| model-provider:groq | 0 | 1 | 1 | 123 | 0 |  |
| model-provider:nvidia-nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
