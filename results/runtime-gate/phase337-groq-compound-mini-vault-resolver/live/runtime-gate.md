# Runtime provider gate campaign

- Name: `phase337-groq-compound-mini-vault-resolver`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam-seeded`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `true`
- Provider latency: `1.341171313s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 142
- Response SHA-256: `b00b36b1a500dc63f96991c046cf3487a68662ed1919428a850f6b9b38f2b12c`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-29T00:25:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-29T00:25:11Z |
| model-binding:groq-compound-mini | 0 | 1 | 1 | 1097 | 0 |  |
| model-provider:groq | 0 | 1 | 1 | 1097 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
