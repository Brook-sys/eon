# Runtime provider gate campaign

- Name: `phase325-groq-allam-active-circuit-max-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam-seeded`
- Selected route: `groq` / `groq-allam`
- Provider success: `true`
- Provider latency: `600.034156ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 117
- Response SHA-256: `37064615980624c6490d7707afa4c07634f62cbad9bd7047dcdda9baba36fd91`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 3/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T13:44:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam | 0 | 1 | 1 | 377 | 0 |  |
| model-binding:groq-allam-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T13:44:45Z |
| model-provider:groq | 0 | 1 | 1 | 377 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
