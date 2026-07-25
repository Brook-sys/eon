# Runtime provider gate campaign

- Name: `phase206-qwen36-json-ceiling-256`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-mistral-small-4-seeded`
- Selected route: `groq` / `groq-qwen36-27b`
- Provider success: `true`
- Provider latency: `741.452784ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 938
- Response SHA-256: `d3d934b0a75677f313e6d7b6fb20a2a5c17a5d4f2e630c6332fe3a653fdcc065`
- Expected response configured: `true`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: `expected_with_prefix_and_suffix`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T04:14:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen36-27b | 0 | 1 | 1 | 346 | 0 |  |
| model-binding:nim-mistral-small-4-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T04:14:30Z |
| model-provider:groq | 0 | 1 | 1 | 346 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
