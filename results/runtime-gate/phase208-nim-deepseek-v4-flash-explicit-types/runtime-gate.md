# Runtime provider gate campaign

- Name: `phase208-nim-deepseek-v4-flash-explicit-types`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-70b-seeded`
- Selected route: `nim` / `nim-deepseek-v4-flash`
- Provider success: `true`
- Provider latency: `11.372355634s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 478
- Response SHA-256: `ecbfd9d87c6bb9053533f5bd76ce578693cfad9be239de2cf541de704f8ca332`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T06:30:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T06:30:38Z |
| model-binding:nim-deepseek-v4-flash | 0 | 1 | 1 | 1122 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 1122 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
