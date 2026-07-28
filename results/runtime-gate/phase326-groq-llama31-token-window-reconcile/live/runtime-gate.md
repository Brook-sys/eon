# Runtime provider gate campaign

- Name: `phase326-groq-llama31-token-window-reconcile`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama31-seeded`
- Selected route: `groq` / `groq-llama31`
- Provider success: `true`
- Provider latency: `327.817446ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 137
- Response SHA-256: `a8ac846a3c28c8bddcc4b97080149eed4171b391c28e4900e331ccd28e63a79a`
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
| model-binding:groq-llama31 | 0 | 1 | 1 | 260 | 0 |  |
| model-binding:groq-llama31-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T15:10:36Z |
| model-provider:groq | 0 | 1 | 1 | 260 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
