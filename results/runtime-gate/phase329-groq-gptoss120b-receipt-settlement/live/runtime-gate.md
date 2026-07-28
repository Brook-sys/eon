# Runtime provider gate campaign

- Name: `phase329-groq-gptoss120b-receipt-settlement`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-gptoss120b-seeded`
- Selected route: `groq` / `groq-gptoss120b`
- Provider success: `false`
- Provider latency: `790.310031ms`
- Provider error class: `provider`
- Provider error reason: `empty_content`
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
| model-binding:groq-gptoss120b | 0 | 1 | 1 | 128 | 1 | 2026-07-28T17:57:19Z |
| model-binding:groq-gptoss120b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T17:57:18Z |
| model-provider:groq | 0 | 1 | 1 | 128 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
