# Runtime provider gate campaign

- Name: `phase332-groq-allam-receipt-alert-failclosed`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-seeded`
- Selected route: `groq` / `groq-allam2`
- Provider success: `true`
- Provider latency: `448.976122ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 194
- Response SHA-256: `d2293888abea7857ac73b225980b78bda3762786b9fe446079d5020cd008a663`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 6/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T19:47:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam2 | 0 | 1 | 1 | 309 | 0 |  |
| model-binding:groq-llama33-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T19:47:14Z |
| model-provider:groq | 0 | 1 | 1 | 309 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
