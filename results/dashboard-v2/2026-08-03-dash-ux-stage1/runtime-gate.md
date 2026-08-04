# Runtime provider gate campaign

- Name: `phase367-groq-gptoss20b-vault-batch-copy-expiring`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-gptoss-20b`
- Selected route: `nim` / `nim-llama33-70b-control`
- Provider success: `false`
- Provider latency: `44.964548507s`
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
- Budget retry attempted: `false`
- Budget retry output tokens: 0
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-gptoss-20b | 0 | 0 | 0 | 0 | 1 | 2026-08-04T00:04:18Z |
| model-binding:nim-llama33-70b-control | 0 | 1 | 1 | 64 | 1 | 2026-08-04T00:06:02Z |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 64 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
