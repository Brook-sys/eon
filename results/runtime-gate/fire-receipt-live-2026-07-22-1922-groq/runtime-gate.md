# Runtime provider gate campaign

- Name: `nim-primary-circuit-to-groq-json-anti-fence-2026-07-22-1540`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-primary`
- Selected route: `groq` / `groq-fallback`
- Provider success: `true`
- Provider latency: `364.70604ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 29
- Response SHA-256: `a261b110a624386dc9e6bb10e71e983ce97f65d38791b95a81c80de176206dc7`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T22:27:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-fallback | 0 | 1 | 1 | 125 | 0 |  |
| model-binding:nim-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-22T22:27:34Z |
| model-provider:groq | 0 | 1 | 1 | 125 | 0 |  |
| model-provider:nvidia-nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
