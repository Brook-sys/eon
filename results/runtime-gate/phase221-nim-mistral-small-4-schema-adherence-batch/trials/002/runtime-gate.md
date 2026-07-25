# Runtime provider gate campaign

- Name: `phase221-nim-mistral-small-4-schema-adherence-batch`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama31-8b-seeded`
- Selected route: `nim` / `nim-mistral-small-4`
- Provider success: `true`
- Provider latency: `3.011110959s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 522
- Response SHA-256: `1c0e95faee554ecbc2cef2ec456ffd6a812d567751b1b85b26114c4fbfe9c845`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T18:44:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T18:44:21Z |
| model-binding:nim-mistral-small-4 | 0 | 1 | 1 | 766 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 766 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
