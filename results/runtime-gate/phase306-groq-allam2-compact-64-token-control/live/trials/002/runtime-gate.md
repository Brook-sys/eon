# Runtime provider gate campaign

- Name: `phase306-groq-allam2-compact-64-token-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-allam2-seeded`
- Selected route: `groq` / `groq-allam2`
- Provider success: `true`
- Provider latency: `361.278136ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 141
- Response SHA-256: `653f345c729b1fa0faaefe6235301765200adb1839adf740f0641d19b5cd9ed7`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 0/0/0
- Response JSON valid: `false`
- Response framing class: `invalid_json`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam2 | 0 | 1 | 1 | 239 | 0 |  |
| model-binding:groq-allam2-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T05:43:24Z |
| model-provider:groq | 0 | 1 | 1 | 239 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
