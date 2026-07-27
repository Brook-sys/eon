# Runtime provider gate campaign

- Name: `phase286-groq-llama33-reconcile-safe-disposition`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-70b-seeded`
- Selected route: `groq` / `groq-llama33-70b-live`
- Provider success: `true`
- Provider latency: `482.065116ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 157
- Response SHA-256: `40609c0f726fa4117d9c041bf0a55a84736401007e40da6d6855d92e26935db2`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T23:08:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b-live | 0 | 1 | 1 | 264 | 0 |  |
| model-binding:nim-llama31-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T23:08:07Z |
| model-provider:groq | 0 | 1 | 1 | 264 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
