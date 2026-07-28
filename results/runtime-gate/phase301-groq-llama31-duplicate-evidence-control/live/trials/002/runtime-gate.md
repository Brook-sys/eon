# Runtime provider gate campaign

- Name: `phase301-groq-llama31-duplicate-evidence-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama31-seeded`
- Selected route: `groq` / `groq-llama31`
- Provider success: `true`
- Provider latency: `236.660833ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 176
- Response SHA-256: `41a0be04d11a966c501fa33f17d928fd82656e53d3af9f23de0c7d3f65ab5f69`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 4/0/0
- Response JSON valid: `false`
- Response framing class: `markdown_fence`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31 | 0 | 1 | 1 | 300 | 0 |  |
| model-binding:groq-llama31-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T04:03:31Z |
| model-provider:groq | 0 | 1 | 1 | 300 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
