# Runtime provider gate campaign

- Name: `phase149-terminal-capacity-control-groq-llama31-8b`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-unused`
- Selected route: `groq` / `groq-llama31-8b`
- Provider success: `true`
- Provider latency: `358.05724ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 26
- Response SHA-256: `745c460f2452a0e7dcb473b9ad58bac9b4ca3b938b8593d439edfa10473a02fe`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-23T12:26:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b | 0 | 1 | 1 | 122 | 0 |  |
| model-binding:nim-unused | 0 | 0 | 0 | 0 | 1 | 2026-07-23T12:25:21Z |
| model-provider:groq | 0 | 1 | 1 | 122 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
