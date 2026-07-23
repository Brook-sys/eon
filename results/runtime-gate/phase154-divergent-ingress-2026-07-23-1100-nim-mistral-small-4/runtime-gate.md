# Runtime provider gate campaign

- Name: `phase154-divergent-ingress-control-nim-mistral-small-4`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-unused`
- Selected route: `nim` / `nim-mistral-small-4`
- Provider success: `true`
- Provider latency: `736.945248ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 34
- Response SHA-256: `13ea4b3095558cb2128dee50f2b24ae00eca9e78ea452cb8068fe7a7e1e058b9`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-23T14:04:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-unused | 0 | 0 | 0 | 0 | 1 | 2026-07-23T14:03:05Z |
| model-binding:nim-mistral-small-4 | 0 | 1 | 1 | 121 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 121 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
