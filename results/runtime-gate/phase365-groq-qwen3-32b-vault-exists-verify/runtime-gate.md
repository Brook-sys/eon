# Runtime provider gate campaign

- Name: `phase365-groq-qwen3-32b-vault-exists-verify`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-qwen3-32b`
- Selected route: `nim` / `nim-llama31-8b-seeded`
- Provider success: `true`
- Provider latency: `623.013431ms`
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
- Second acquire: `resource_resource_rate_limit` until `2026-08-01T16:46:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen3-32b | 0 | 0 | 0 | 0 | 1 | 2026-08-01T16:45:35Z |
| model-binding:nim-llama31-8b-seeded | 0 | 1 | 1 | 95 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 95 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
