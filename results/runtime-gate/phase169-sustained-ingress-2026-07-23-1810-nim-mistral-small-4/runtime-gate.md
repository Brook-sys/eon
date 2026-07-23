# Runtime provider gate campaign

- Name: `phase169-sustained-ingress-nim-mistral-small-4`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-unused`
- Selected route: `nim` / `nim-mistral-small-4`
- Provider success: `true`
- Provider latency: `838.216608ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 35
- Response SHA-256: `447b6350e99953872e1ae8d3d4ec974296322503621637c69e3860d908c56dd4`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `false`
- Response framing class: ``
- Second acquire: `resource_resource_rate_limit` until `2026-07-23T21:07:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-unused | 0 | 0 | 0 | 0 | 1 | 2026-07-23T21:06:35Z |
| model-binding:nim-mistral-small-4 | 0 | 1 | 1 | 139 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 139 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
