# Runtime provider gate campaign

- Name: `phase328-groq-compound-mini-completion-receipt-boundary`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-compound-mini-seeded`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `true`
- Provider latency: `1.280231033s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 161
- Response SHA-256: `aa0f9d1d5e0e5afa937462c7ab339024948e2f9ffe642da2a81d900392b80e52`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T17:08:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-compound-mini | 0 | 1 | 1 | 1033 | 0 |  |
| model-binding:groq-compound-mini-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T17:08:49Z |
| model-provider:groq | 0 | 1 | 1 | 1033 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
