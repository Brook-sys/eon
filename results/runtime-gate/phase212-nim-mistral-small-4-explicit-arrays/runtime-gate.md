# Runtime provider gate campaign

- Name: `phase212-nim-mistral-small-4-explicit-arrays`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama31-8b-seeded`
- Selected route: `nim` / `nim-mistral-small-4`
- Provider success: `true`
- Provider latency: `2.989409832s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 521
- Response SHA-256: `e8c1ed0da20e2c41cf0e9c7a6c31d525d16f73016c71d068941f448f93d07a0f`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T07:50:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T07:50:48Z |
| model-binding:nim-mistral-small-4 | 0 | 1 | 1 | 734 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 734 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
