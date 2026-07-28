# Runtime provider gate campaign

- Name: `phase310-groq-qwen36-27b-duplicate-evidence`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-qwen36-seeded`
- Selected route: `groq` / `groq-qwen36`
- Provider success: `true`
- Provider latency: `636.442171ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 485
- Response SHA-256: `32c66be679b141292e0b45e271991f436f8f1267215ace3a0587a8d1f9a04ab6`
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
| model-binding:groq-qwen36 | 0 | 1 | 1 | 271 | 0 |  |
| model-binding:groq-qwen36-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T07:05:53Z |
| model-provider:groq | 0 | 1 | 1 | 271 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
