# Runtime provider gate campaign

- Name: `phase220-groq-llama31-8b-schema-adherence-batch`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-meta-llama31-8b-seeded`
- Selected route: `groq` / `groq-llama31-8b`
- Provider success: `true`
- Provider latency: `527.452407ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 515
- Response SHA-256: `e8323bc1849e9be0685dc89ae502529efce4142ec92d9ed55d22a9130e52a732`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T15:08:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama31-8b | 0 | 1 | 1 | 752 | 0 |  |
| model-binding:nim-meta-llama31-8b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T15:08:27Z |
| model-provider:groq | 0 | 1 | 1 | 752 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
