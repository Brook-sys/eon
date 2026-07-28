# Runtime provider gate campaign

- Name: `phase319-nim-llama31-conflicting-retry-after-control`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-seeded`
- Selected route: `nim` / `nim-llama31`
- Provider success: `true`
- Provider latency: `20.916605781s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 136
- Response SHA-256: `58bda7d4180112b50615c099c7d2f233dc8f9bbec3bed53a26299381284a85af`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T10:05:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:nim-llama31 | 0 | 1 | 1 | 262 | 0 |  |
| model-binding:nim-llama31-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T10:05:22Z |
| model-provider:nim | 0 | 1 | 1 | 262 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
