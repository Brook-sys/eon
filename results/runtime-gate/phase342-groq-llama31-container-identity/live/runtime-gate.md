# Runtime provider gate campaign

- Name: `phase342-groq-llama31-container-identity`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-compound-mini-seeded`
- Selected route: `groq` / `groq-llama31`
- Provider success: `true`
- Provider latency: `403.978708ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 172
- Response SHA-256: `4ba05fe24deb7e08ef0899514ba77220905929a4dd2fa92323737e6fc66b722e`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 6/0/0
- Response JSON valid: `false`
- Response framing class: `markdown_fence`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-compound-mini-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-29T02:14:19Z |
| model-binding:groq-llama31 | 0 | 1 | 1 | 265 | 0 |  |
| model-provider:groq | 0 | 1 | 1 | 265 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
