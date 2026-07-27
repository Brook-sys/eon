# Runtime provider gate campaign

- Name: `phase242-groq-llama33-70b-exact-json-authority-free`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-mistral-small-4-seeded`
- Selected route: `groq` / `groq-llama33-70b`
- Provider success: `true`
- Provider latency: `184.528241ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 18
- Response SHA-256: `11bec2db0e99977c840a5df4b17be528421aadd5f4aa79d3622c126c481397fb`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T03:23:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b | 0 | 1 | 1 | 120 | 0 |  |
| model-binding:nim-mistral-small-4-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T03:23:28Z |
| model-provider:groq | 0 | 1 | 1 | 120 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
