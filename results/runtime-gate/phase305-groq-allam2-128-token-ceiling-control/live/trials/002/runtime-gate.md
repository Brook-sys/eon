# Runtime provider gate campaign

- Name: `phase305-groq-allam2-128-token-ceiling-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam2-seeded`
- Selected route: `groq` / `groq-allam2`
- Provider success: `true`
- Provider latency: `389.144527ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 181
- Response SHA-256: `b84320490946afef457f57a03a5009144ac0f3adcffe1dcd179b4df81133ca0e`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 4/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T05:23:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam2 | 0 | 1 | 1 | 401 | 0 |  |
| model-binding:groq-allam2-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T05:23:46Z |
| model-provider:groq | 0 | 1 | 1 | 401 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
