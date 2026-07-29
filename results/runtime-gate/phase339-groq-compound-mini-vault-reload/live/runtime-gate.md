# Runtime provider gate campaign

- Name: `phase339-groq-compound-mini-vault-reload`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-seeded`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `true`
- Provider latency: `1.138611905s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 161
- Response SHA-256: `ed7c22a3a09f68b61aa73c660ce3004d0a5fb1c92e183cc22c4a9e5e640083d0`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 6/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-29T01:23:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-compound-mini | 0 | 1 | 1 | 1041 | 0 |  |
| model-binding:groq-llama33-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-29T01:23:27Z |
| model-provider:groq | 0 | 1 | 1 | 1041 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
