# Runtime provider gate campaign

- Name: `phase312-groq-qwen36-1024-token-boundary`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-qwen36-seeded`
- Selected route: `groq` / `groq-qwen36`
- Provider success: `true`
- Provider latency: `2.381427819s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 4131
- Response SHA-256: `cad86b9935d6d3d4ad9ff6749f0d3ac8c4010ee4be444ed59d2be620bcfb22d9`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 0/0/0
- Response JSON valid: `false`
- Response framing class: `expected_with_prefix_and_suffix`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen36 | 0 | 1 | 1 | 1167 | 0 |  |
| model-binding:groq-qwen36-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T07:07:54Z |
| model-provider:groq | 0 | 1 | 1 | 1167 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
