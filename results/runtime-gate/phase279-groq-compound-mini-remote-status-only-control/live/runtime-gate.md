# Runtime provider gate campaign

- Name: `phase279-groq-compound-mini-remote-status-only-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-compound-mini-seeded`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `true`
- Provider latency: `1.322343036s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 107
- Response SHA-256: `3613a555d9fadc92234d9b1159e5b2f6a23f1e718e760e07e204364157537d0b`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 4/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T20:03:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-compound-mini | 0 | 1 | 1 | 970 | 0 |  |
| model-binding:groq-compound-mini-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T20:03:51Z |
| model-provider:groq | 0 | 1 | 1 | 970 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
