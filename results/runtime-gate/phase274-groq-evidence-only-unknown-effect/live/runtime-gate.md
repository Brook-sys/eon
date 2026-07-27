# Runtime provider gate campaign

- Name: `phase274-groq-evidence-only-unknown-effect`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-primary`
- Selected route: `groq` / `groq-llama33-70b-versatile`
- Provider success: `true`
- Provider latency: `479.257635ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 109
- Response SHA-256: `145f47abb37996ea18d87c0a412ba5a90d419ce0450b96319f47343c8b0c4676`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 3/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T17:43:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b-versatile | 0 | 1 | 1 | 190 | 0 |  |
| model-binding:groq-seeded-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-27T17:43:28Z |
| model-provider:groq | 0 | 1 | 1 | 190 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
