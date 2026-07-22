# Runtime provider gate campaign

- Name: `groq-primary-circuit-to-nim-minimal-exact-text-2026-07-22-1320`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-primary`
- Selected route: `nvidia-nim` / `nim-fallback`
- Provider success: `true`
- Provider latency: `640.566325ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 2
- Response SHA-256: `565339bc4d33d72817b583024112eb7f5cdf3e5eef0252d6ec1b9c9a94e12bb3`
- Expected response configured: `true`
- Expected response exact match: `true`
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T16:23:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-22T16:23:29Z |
| model-binding:nim-fallback | 0 | 1 | 1 | 68 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nvidia-nim | 0 | 1 | 1 | 68 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
