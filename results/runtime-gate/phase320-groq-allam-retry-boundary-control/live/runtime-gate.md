# Runtime provider gate campaign

- Name: `phase320-groq-allam-retry-boundary-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam-seeded`
- Selected route: `groq` / `groq-allam`
- Provider success: `true`
- Provider latency: `426.695566ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 95
- Response SHA-256: `916c93751bdb56d4d26f23e54090a8313292410a15a33135a15dc1d56c435e14`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 3/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T10:30:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam | 0 | 1 | 1 | 362 | 0 |  |
| model-binding:groq-allam-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T10:30:28Z |
| model-provider:groq | 0 | 1 | 1 | 362 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
