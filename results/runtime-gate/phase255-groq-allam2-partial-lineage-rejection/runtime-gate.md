# Runtime provider gate campaign

- Name: `phase255-groq-allam2-partial-lineage-rejection`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded`
- Selected route: `groq` / `groq-allam2-7b`
- Provider success: `true`
- Provider latency: `578.638884ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 542
- Response SHA-256: `ec60738f90455d6f0411a34dc3fdf0846aa6658d07b79819dac056f4db141049`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam2-7b | 0 | 1 | 1 | 1022 | 0 |  |
| model-binding:nim-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T11:10:19Z |
| model-provider:groq | 0 | 1 | 1 | 1022 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
