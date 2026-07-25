# Runtime provider gate campaign

- Name: `phase217-mixed-batch-success-failure`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-mistral-small-4-seeded`
- Selected route: `groq` / `groq-qwen36-27b`
- Provider success: `true`
- Provider latency: `1.064717533s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 1389
- Response SHA-256: `267bbd7e2526ea1dfeef765b421ef992913d577ea74b2b0601db45fc16b1b386`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: `invalid_json`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen36-27b | 0 | 1 | 1 | 987 | 0 |  |
| model-binding:nim-mistral-small-4-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T10:46:24Z |
| model-provider:groq | 0 | 1 | 1 | 987 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
