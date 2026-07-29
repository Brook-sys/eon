# Runtime provider gate campaign

- Name: `phase351-groq-llama33-vault-import-options-classify`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-70b-classify`
- Selected route: `nim` / `nim-llama31-70b-control`
- Provider success: `true`
- Provider latency: `4.009781161s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 244
- Response SHA-256: `ad058de2ad2109f9100ba2883c1042a0ec5df10cfaf0bb582eb718053fa23742`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `false`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 0/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-29T14:50:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b-classify | 0 | 0 | 0 | 0 | 1 | 2026-07-29T14:50:51Z |
| model-binding:nim-llama31-70b-control | 0 | 1 | 1 | 275 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 275 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
