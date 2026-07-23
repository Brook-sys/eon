# Runtime provider gate campaign

- Name: `phase135-receipt-replay-control-groq-llama33-70b`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-unused`
- Selected route: `groq` / `groq-llama33-70b`
- Provider success: `true`
- Provider latency: `362.407878ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 23
- Response SHA-256: `8aae477137a9d696dab5173d4d82cd2bcc52a585ce7dfb58b81a233762a9613f`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-23T08:25:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b | 0 | 1 | 1 | 125 | 0 |  |
| model-binding:nim-unused | 0 | 0 | 0 | 0 | 1 | 2026-07-23T08:24:52Z |
| model-provider:groq | 0 | 1 | 1 | 125 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
