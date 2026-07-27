# Runtime provider gate campaign

- Name: `phase283-nim-llama33-complete-evidence-pair-control`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama33-70b-seeded`
- Selected route: `nim` / `nim-llama33-70b-live`
- Provider success: `false`
- Provider latency: `44.929728885s`
- Provider error class: `provider`
- Provider error reason: ``
- Provider HTTP status: 0
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
| model-binding:nim-llama33-70b-live | 0 | 1 | 1 | 64 | 1 | 2026-07-27T22:11:27Z |
| model-binding:nim-llama33-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T22:10:42Z |
| model-provider:nim | 0 | 1 | 1 | 64 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
