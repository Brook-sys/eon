# Runtime provider gate campaign

- Name: `heartbeat-test`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama-3.3-70b`
- Selected route: `nvidia-nim` / `nvidia-mistral-small-4`
- Provider success: `true`
- Provider latency: `626.988586ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 2
- Response SHA-256: `565339bc4d33d72817b583024112eb7f5cdf3e5eef0252d6ec1b9c9a94e12bb3`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `false`
- Response framing class: ``
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T20:22:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama-3.3-70b | 0 | 0 | 0 | 0 | 1 | 2026-07-22T20:22:53Z |
| model-binding:nvidia-mistral-small-4 | 0 | 1 | 1 | 63 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nvidia-nim | 0 | 1 | 1 | 63 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
