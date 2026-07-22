# Runtime provider gate campaign

- Name: `groq-primary-circuit-to-nim-finish-reason-2026-07-22-1240`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-primary`
- Selected route: `nvidia-nim` / `nim-fallback`
- Provider success: `true`
- Provider latency: `1.275241985s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 212
- Response SHA-256: `55c59e491d251d101287c67fadecea36239eb389e6fae542daf76c5fde3bbd6c`
- Expected response configured: `true`
- Expected response exact match: `false`
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T15:45:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-22T15:45:34Z |
| model-binding:nim-fallback | 0 | 1 | 1 | 303 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nvidia-nim | 0 | 1 | 1 | 303 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
