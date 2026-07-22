# Runtime provider gate campaign

- Name: `nim-primary-circuit-to-groq-healthy-2026-07-22-1220`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-primary`
- Selected route: `groq` / `groq-fallback`
- Provider success: `true`
- Provider latency: `346.475009ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Response bytes: 122
- Response SHA-256: `06d0c73637fad7d2f31d2b13adbdfed302bac43c611d62db16570ead78ee63c6`
- Expected response configured: `true`
- Expected response exact match: `false`
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T15:25:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-fallback | 0 | 1 | 1 | 287 | 0 |  |
| model-binding:nim-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-22T15:25:10Z |
| model-provider:groq | 0 | 1 | 1 | 287 | 0 |  |
| model-provider:nvidia-nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
