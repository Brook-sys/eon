# Runtime provider gate campaign

- Name: `phase231-groq-compound-mini-provenance`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-mistral-small-4-seeded`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `false`
- Provider latency: `1.196669618s`
- Provider error class: `http`
- Provider HTTP status: 429
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
| model-binding:groq-compound-mini | 0 | 1 | 1 | 384 | 1 | 2026-07-26T23:27:32Z |
| model-binding:nim-mistral-small-4-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-26T23:27:31Z |
| model-provider:groq | 0 | 1 | 1 | 384 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
