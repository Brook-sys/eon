# Runtime provider gate campaign

- Name: `phase177-synchronous-pragma-nim-mistral-small-4`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-mistral-small-4`
- Selected route: `groq` / `groq-unused`
- Provider success: `true`
- Provider latency: `409.898313ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 27
- Response SHA-256: `af1fc466cfcdcf7739f3d44b846f297d4390e962a050007f18f4bc1a538b27b5`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T01:25:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-unused | 0 | 1 | 1 | 152 | 0 |  |
| model-binding:nim-mistral-small-4 | 0 | 0 | 0 | 0 | 1 | 2026-07-24T01:24:47Z |
| model-provider:groq | 0 | 1 | 1 | 152 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
