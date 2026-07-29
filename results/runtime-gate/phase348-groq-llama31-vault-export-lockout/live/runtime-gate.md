# Runtime provider gate campaign

- Name: `phase348-groq-llama31-vault-export-lockout`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-seeded`
- Selected route: `groq` / `groq-llama31`
- Provider success: `true`
- Provider latency: `403.683478ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 203
- Response SHA-256: `c6f584c3e0e8a44c3e2103d9a9f005899c23977272fd639c75099bc64f5c4bcf`
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
| model-binding:groq-llama31 | 0 | 1 | 1 | 275 | 0 |  |
| model-binding:nim-llama31-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-29T11:26:43Z |
| model-provider:groq | 0 | 1 | 1 | 275 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
