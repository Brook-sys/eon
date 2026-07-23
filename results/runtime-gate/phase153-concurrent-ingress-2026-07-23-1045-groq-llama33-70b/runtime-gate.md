# Runtime provider gate campaign

- Name: `phase153-concurrent-ingress-control-groq-llama33-70b`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-unused`
- Selected route: `groq` / `groq-llama33-70b`
- Provider success: `true`
- Provider latency: `271.599108ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 31
- Response SHA-256: `71ba14e8f045a94eeccb35c63cd54f8488c28f7e932fb8e4440131b5a4a0d1ba`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-23T13:44:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b | 0 | 1 | 1 | 134 | 0 |  |
| model-binding:nim-unused | 0 | 0 | 0 | 0 | 1 | 2026-07-23T13:43:20Z |
| model-provider:groq | 0 | 1 | 1 | 134 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
