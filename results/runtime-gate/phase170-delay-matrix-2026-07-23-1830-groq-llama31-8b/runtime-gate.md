# Runtime provider gate campaign

- Name: `phase170-delay-matrix-groq-llama31-8b`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-unused`
- Selected route: `groq` / `groq-llama31-8b`
- Provider success: `true`
- Provider latency: `281.289885ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 29
- Response SHA-256: `b51f27b7309edfb543efa42852134b9ef5bc507b3f71b3edc0986c173671f402`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `false`
- Response framing class: ``
- Second acquire: `resource_resource_rate_limit` until `2026-07-23T21:24:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b | 0 | 1 | 1 | 151 | 0 |  |
| model-binding:nim-unused | 0 | 0 | 0 | 0 | 1 | 2026-07-23T21:23:32Z |
| model-provider:groq | 0 | 1 | 1 | 151 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
