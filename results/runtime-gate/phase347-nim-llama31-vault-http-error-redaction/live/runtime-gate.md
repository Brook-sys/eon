# Runtime provider gate campaign

- Name: `phase347-nim-llama31-vault-http-error-redaction`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam2-seeded`
- Selected route: `nvidia_nim` / `nim-llama31`
- Provider success: `true`
- Provider latency: `1.282510049s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 230
- Response SHA-256: `731d2ed020882eb1601bbc3a80499eed671e36e6bff13c64778a45024ecf70ed`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 7/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-29T09:31:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam2-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-29T09:31:23Z |
| model-binding:nim-llama31 | 0 | 1 | 1 | 299 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nvidia_nim | 0 | 1 | 1 | 299 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
