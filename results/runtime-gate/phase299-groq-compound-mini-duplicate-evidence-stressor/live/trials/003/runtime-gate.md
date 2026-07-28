# Runtime provider gate campaign

- Name: `phase299-groq-compound-mini-duplicate-evidence-stressor`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-compound-mini-seeded`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `false`
- Provider latency: `274.661725ms`
- Provider error class: `http`
- Provider error reason: ``
- Provider HTTP status: 429
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
| model-binding:groq-compound-mini | 0 | 1 | 1 | 64 | 1 | 2026-07-28T03:23:52Z |
| model-binding:groq-compound-mini-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T03:23:51Z |
| model-provider:groq | 0 | 1 | 1 | 64 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
