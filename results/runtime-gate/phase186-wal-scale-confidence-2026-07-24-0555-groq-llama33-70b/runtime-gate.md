# Runtime provider gate campaign

- Name: `phase186-wal-scale-confidence-groq-llama33-70b`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded-open`
- Selected route: `groq` / `groq-llama33-70b`
- Provider success: `true`
- Provider latency: `296.651512ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 29
- Response SHA-256: `7f6dfc4a540298fe301dd6d37b072330625ad6a2f201f0e6e07d2757e9e93a61`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T08:46:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b | 0 | 1 | 1 | 141 | 0 |  |
| model-binding:nim-seeded-open | 0 | 0 | 0 | 0 | 1 | 2026-07-24T08:45:25Z |
| model-provider:groq | 0 | 1 | 1 | 141 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
