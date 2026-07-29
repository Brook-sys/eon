# Runtime provider gate campaign

- Name: `phase354-groq-qwen25-vault-single-resolve`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-8b-seeded`
- Selected route: `groq` / `groq-qwen25-32b`
- Provider success: `false`
- Provider latency: `135.177541ms`
- Provider error class: `http`
- Provider error reason: ``
- Provider HTTP status: 400
- Provider Retry-After: `0s`
- Finish reason: ``
- Response bytes: 0
- Response SHA-256: ``
- Expected response configured: `false`
- Expected response exact match: `false`
- Structural comparison configured: `false`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 0/0/0
- Response JSON valid: `false`
- Response framing class: ``
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen25-32b | 0 | 1 | 1 | 32 | 1 | 2026-07-29T18:23:10Z |
| model-binding:nim-llama31-8b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-29T18:23:09Z |
| model-provider:groq | 0 | 1 | 1 | 32 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
