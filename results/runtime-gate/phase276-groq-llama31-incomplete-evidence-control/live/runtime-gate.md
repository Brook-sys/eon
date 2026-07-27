# Runtime provider gate campaign

- Name: `phase276-groq-llama31-incomplete-evidence-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-primary`
- Selected route: `groq` / `groq-llama31-8b`
- Provider success: `true`
- Provider latency: `339.696062ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 139
- Response SHA-256: `2a0e28371f60607e83c358a1f8b300d6e5a90dfb6f7205187e4fff6fb688a56a`
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
| model-binding:groq-llama31-8b | 0 | 1 | 1 | 231 | 0 |  |
| model-binding:groq-seeded-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-27T18:44:03Z |
| model-provider:groq | 0 | 1 | 1 | 231 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
