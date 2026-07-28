# Runtime provider gate campaign

- Name: `phase298-groq-llama33-explicit-array-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-70b-seeded`
- Selected route: `groq` / `groq-llama33-70b`
- Provider success: `true`
- Provider latency: `338.422998ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 133
- Response SHA-256: `54382d263f86826433526eee9e44450d954f67c165b01feddca79d1c4177503f`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 4/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T03:03:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b | 0 | 1 | 1 | 249 | 0 |  |
| model-binding:groq-llama33-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T03:03:23Z |
| model-provider:groq | 0 | 1 | 1 | 249 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
