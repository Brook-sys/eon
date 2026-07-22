# Runtime provider gate campaign

- Name: `groq-primary-circuit-to-nim-minimal-exact-json-2026-07-22-1340`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-primary`
- Selected route: `nvidia-nim` / `nim-fallback`
- Provider success: `true`
- Provider latency: `838.340121ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 29
- Response SHA-256: `a261b110a624386dc9e6bb10e71e983ce97f65d38791b95a81c80de176206dc7`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T16:46:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-22T16:46:08Z |
| model-binding:nim-fallback | 0 | 1 | 1 | 93 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nvidia-nim | 0 | 1 | 1 | 93 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
