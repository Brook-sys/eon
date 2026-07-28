# Runtime provider gate campaign

- Name: `phase330-groq-llama31-runtime-recovery`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama31-seeded`
- Selected route: `groq` / `groq-llama31`
- Provider success: `true`
- Provider latency: `413.007557ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 167
- Response SHA-256: `fb82e30cf11995374f098cc3b0703aa972a1cb43a9f4f249beeabede037a11d6`
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
| model-binding:groq-llama31 | 0 | 1 | 1 | 269 | 0 |  |
| model-binding:groq-llama31-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T18:15:20Z |
| model-provider:groq | 0 | 1 | 1 | 269 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
