# Runtime provider gate campaign

- Name: `phase375-groq-qwen36-vault-batch-history`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-qwen36-27b`
- Selected route: `nim` / `nim-llama31-8b-instruct`
- Provider success: `true`
- Provider latency: `811.513471ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 5
- Response SHA-256: `c2e3ac47f4a325469c1a2d5f117e463ec943c721986d5d9f09ac4540b7d80526`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `false`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 0/0/0
- Response JSON valid: `false`
- Response framing class: ``
- Budget retry attempted: `false`
- Budget retry output tokens: 0
- Second acquire: `resource_resource_rate_limit` until `2026-08-04T15:27:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen36-27b | 0 | 0 | 0 | 0 | 1 | 2026-08-04T15:26:09Z |
| model-binding:nim-llama31-8b-instruct | 0 | 1 | 1 | 95 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 95 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
