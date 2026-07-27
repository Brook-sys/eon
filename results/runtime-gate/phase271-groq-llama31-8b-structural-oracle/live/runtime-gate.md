# Runtime provider gate campaign

- Name: `phase271-groq-llama31-8b-structural-oracle`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded`
- Selected route: `groq` / `groq-llama31-8b-instant`
- Provider success: `true`
- Provider latency: `391.904372ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 82
- Response SHA-256: `c4def39bb58b5b2c5cff2e26355d09b71cc5a42f37d601f0daf4e4c3f1cb1de8`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 2/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T16:11:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b-instant | 0 | 1 | 1 | 137 | 0 |  |
| model-binding:nim-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T16:11:25Z |
| model-provider:groq | 0 | 1 | 1 | 137 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
