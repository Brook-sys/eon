# Runtime provider gate campaign

- Name: `phase344-groq-compound-mini-child-digest-platform-identity`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam2-seeded`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `true`
- Provider latency: `1.334460256s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 196
- Response SHA-256: `260e68169e0f7228b0e768ad55846b223972448f106a1cdc1142b20bf54a99cc`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 7/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-29T06:26:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam2-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-29T06:26:47Z |
| model-binding:groq-compound-mini | 0 | 1 | 1 | 1155 | 0 |  |
| model-provider:groq | 0 | 1 | 1 | 1155 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
