# Runtime provider gate campaign

- Name: `phase333-groq-llama33-receipt-query-http-failclosed`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam-seeded`
- Selected route: `groq` / `groq-llama33`
- Provider success: `true`
- Provider latency: `473.343095ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 151
- Response SHA-256: `a0216155e7eb36d9873720b41c318df0807cc48b7fcc83b1526df5e0ce92abfb`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 6/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T20:04:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T20:04:03Z |
| model-binding:groq-llama33 | 0 | 1 | 1 | 221 | 0 |  |
| model-provider:groq | 0 | 1 | 1 | 221 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
