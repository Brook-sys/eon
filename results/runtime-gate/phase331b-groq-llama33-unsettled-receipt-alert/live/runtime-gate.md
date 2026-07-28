# Runtime provider gate campaign

- Name: `phase331b-groq-llama33-unsettled-receipt-alert`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-seeded`
- Selected route: `groq` / `groq-llama33`
- Provider success: `true`
- Provider latency: `372.674141ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 109
- Response SHA-256: `02d462fb6f605050aeaf9c2598c3075e9f42cdcdb231dfa11d7315bccc47fca6`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T19:09:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33 | 0 | 1 | 1 | 240 | 0 |  |
| model-binding:nim-llama31-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T19:09:25Z |
| model-provider:groq | 0 | 1 | 1 | 240 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
