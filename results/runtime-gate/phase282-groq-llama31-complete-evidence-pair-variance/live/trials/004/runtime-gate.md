# Runtime provider gate campaign

- Name: `phase282-groq-llama31-complete-evidence-pair-variance`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama31-8b-seeded`
- Selected route: `groq` / `groq-llama31-8b-live`
- Provider success: `true`
- Provider latency: `261.502582ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 163
- Response SHA-256: `0ca7e16bf26aea3e32960df5fc34782da3711d7f74565e2f225ba0378a3cdd65`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 3/1/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T22:10:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b-live | 0 | 1 | 1 | 237 | 0 |  |
| model-binding:groq-llama31-8b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T22:10:43Z |
| model-provider:groq | 0 | 1 | 1 | 237 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
