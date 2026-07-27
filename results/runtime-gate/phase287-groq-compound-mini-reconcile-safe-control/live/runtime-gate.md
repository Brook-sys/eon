# Runtime provider gate campaign

- Name: `phase287-groq-compound-mini-reconcile-safe-control`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-70b-seeded`
- Selected route: `groq` / `groq-compound-mini-live`
- Provider success: `true`
- Provider latency: `1.206586358s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 147
- Response SHA-256: `96f741fbb7a4e7100b2d9eb3282bc8ff18f693f9d80c77a09637fd317221656e`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T23:08:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-compound-mini-live | 0 | 1 | 1 | 1058 | 0 |  |
| model-binding:nim-llama31-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T23:08:30Z |
| model-provider:groq | 0 | 1 | 1 | 1058 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
