# Runtime provider gate campaign

- Name: `phase195-wal-progress-nim-llama31-8b`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-open`
- Selected route: `nim` / `nim-llama31-8b`
- Provider success: `true`
- Provider latency: `756.754016ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 30
- Response SHA-256: `6e4174da52ec0d57c1d3ea1118c1de723c5cf11484467d5af813096bf355f25b`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T18:26:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-seeded-open | 0 | 0 | 0 | 0 | 1 | 2026-07-24T18:25:03Z |
| model-binding:nim-llama31-8b | 0 | 1 | 1 | 143 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 143 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
