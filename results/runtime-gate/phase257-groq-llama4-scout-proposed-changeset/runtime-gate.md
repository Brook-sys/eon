# Runtime provider gate campaign

- Name: `phase257-groq-llama4-scout-proposed-changeset`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded`
- Selected route: `groq` / `groq-llama4-scout-17b`
- Provider success: `false`
- Provider latency: `153.44024ms`
- Provider error class: `http`
- Provider error reason: ``
- Provider HTTP status: 404
- Provider Retry-After: `0s`
- Finish reason: ``
- Response bytes: 0
- Response SHA-256: ``
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: ``
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama4-scout-17b | 0 | 1 | 1 | 768 | 1 | 2026-07-27T11:43:30Z |
| model-binding:nim-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T11:43:30Z |
| model-provider:groq | 0 | 1 | 1 | 768 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
