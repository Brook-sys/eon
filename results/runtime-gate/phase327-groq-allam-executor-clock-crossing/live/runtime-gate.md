# Runtime provider gate campaign

- Name: `phase327-groq-allam-executor-clock-crossing`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam-seeded`
- Selected route: `groq` / `groq-allam`
- Provider success: `true`
- Provider latency: `596.639538ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 168
- Response SHA-256: `3db352f5d4dd4d3133e5d387d54be67cb0bd2f581b2103c5034afbe7129eb1fa`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T16:43:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam | 0 | 1 | 1 | 330 | 0 |  |
| model-binding:groq-allam-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T16:43:42Z |
| model-provider:groq | 0 | 1 | 1 | 330 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
