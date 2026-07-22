# Runtime provider gate campaign

- Name: `nim-primary-circuit-to-groq-minimal-exact-text-2026-07-22-1300`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-primary`
- Selected route: `groq` / `groq-fallback`
- Provider success: `true`
- Provider latency: `268.072724ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 2
- Response SHA-256: `565339bc4d33d72817b583024112eb7f5cdf3e5eef0252d6ec1b9c9a94e12bb3`
- Expected response configured: `true`
- Expected response exact match: `true`
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T16:06:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-fallback | 0 | 1 | 1 | 90 | 0 |  |
| model-binding:nim-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-22T16:06:36Z |
| model-provider:groq | 0 | 1 | 1 | 90 | 0 |  |
| model-provider:nvidia-nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
