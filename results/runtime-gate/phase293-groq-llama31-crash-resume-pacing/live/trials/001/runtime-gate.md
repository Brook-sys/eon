# Runtime provider gate campaign

- Name: `phase293-groq-llama31-crash-resume-pacing`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-70b-seeded`
- Selected route: `groq` / `groq-llama31-8b-live`
- Provider success: `true`
- Provider latency: `318.451999ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 182
- Response SHA-256: `74146199e58ee27ef0abb45fec03a9b3102342eeee3ced91c92979e6fd827764`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `false`
- Response framing class: `markdown_fence`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b-live | 0 | 1 | 1 | 275 | 0 |  |
| model-binding:nim-llama31-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T01:25:21Z |
| model-provider:groq | 0 | 1 | 1 | 275 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
