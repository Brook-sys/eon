# Runtime provider gate campaign

- Name: `phase-125-nim-epistemic-commit-rerun3`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama-3.1-8b`
- Selected route: `nvidia-nim` / `nvidia-mistral-small-4`
- Provider success: `true`
- Provider latency: `3.202290021s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 600
- Response SHA-256: `ce156895c8af67927ca007a8ee0d4556d5d3644e7a5dc8d3611813b94a59936a`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: ``
- Second acquire: `resource_resource_rate_limit` until `2026-07-22T21:48:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama-3.1-8b | 0 | 0 | 0 | 0 | 1 | 2026-07-22T21:48:31Z |
| model-binding:nvidia-mistral-small-4 | 0 | 1 | 1 | 631 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nvidia-nim | 0 | 1 | 1 | 631 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
