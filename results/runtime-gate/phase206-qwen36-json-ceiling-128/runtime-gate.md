# Runtime provider gate campaign

- Name: `phase206-qwen36-json-ceiling-128`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-mistral-small-4-seeded`
- Selected route: `groq` / `groq-qwen36-27b`
- Provider success: `true`
- Provider latency: `671.735674ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 500
- Response SHA-256: `1f83dee5ad2b79f2ba62245616d863cb808102a3563226e2695838039564403a`
- Expected response configured: `true`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: `expected_with_prefix_and_suffix`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T04:13:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen36-27b | 0 | 1 | 1 | 218 | 0 |  |
| model-binding:nim-mistral-small-4-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T04:13:42Z |
| model-provider:groq | 0 | 1 | 1 | 218 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
