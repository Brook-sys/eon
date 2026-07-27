# Runtime provider gate campaign

- Name: `phase278-groq-llama33-remote-status-only-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-primary`
- Selected route: `groq` / `groq-llama33-70b`
- Provider success: `true`
- Provider latency: `507.448552ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 114
- Response SHA-256: `12c1793f840a8d31ab09c8c97ff4f8302dfdf80ea63eb3262212ddc260363e9b`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 4/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T19:23:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b | 0 | 1 | 1 | 221 | 0 |  |
| model-binding:groq-seeded-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-27T19:23:53Z |
| model-provider:groq | 0 | 1 | 1 | 221 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
