# Runtime provider gate campaign

- Name: `phase273-groq-llama33-70b-semantic-structural-control`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded`
- Selected route: `groq` / `groq-llama33-70b-versatile`
- Provider success: `true`
- Provider latency: `364.768528ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 124
- Response SHA-256: `d8036aed986cf0fb92daa8530c20a29480a5540d15706c78a7115ae3ffad1e2f`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 1/1/1
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T17:03:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b-versatile | 0 | 1 | 1 | 189 | 0 |  |
| model-binding:nim-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T17:03:31Z |
| model-provider:groq | 0 | 1 | 1 | 189 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
