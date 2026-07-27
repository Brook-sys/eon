# Runtime provider gate campaign

- Name: `phase275-groq-compound-mini-evidence-contract`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-primary`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `true`
- Provider latency: `1.139091713s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 103
- Response SHA-256: `76299b8d9b8fcf96a247d51043d1e827ff6d1e2dd61688332745311b3b5edca0`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 3/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T18:29:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-compound-mini | 0 | 1 | 1 | 887 | 0 |  |
| model-binding:groq-seeded-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-27T18:29:38Z |
| model-provider:groq | 0 | 1 | 1 | 887 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
