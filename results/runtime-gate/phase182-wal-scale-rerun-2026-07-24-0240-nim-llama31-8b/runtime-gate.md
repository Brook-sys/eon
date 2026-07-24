# Runtime provider gate campaign

- Name: `phase182-wal-scale-rerun-nim-llama31-8b`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-8b`
- Selected route: `groq` / `groq-seeded-open`
- Provider success: `true`
- Provider latency: `459.460728ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 24
- Response SHA-256: `a00577de0cd13c11a51fbb63ccbdc58af2a7d6595ffa907781f0c32ce4bc10e3`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-24T05:48:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-seeded-open | 0 | 1 | 1 | 144 | 0 |  |
| model-binding:nim-llama31-8b | 0 | 0 | 0 | 0 | 1 | 2026-07-24T05:47:58Z |
| model-provider:groq | 0 | 1 | 1 | 144 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
