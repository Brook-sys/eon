# Runtime provider gate campaign

- Name: `phase302-groq-llama33-duplicate-evidence-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-seeded`
- Selected route: `groq` / `groq-llama33`
- Provider success: `true`
- Provider latency: `368.912758ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 154
- Response SHA-256: `47dcb5108c78680cfc4b7eee6fbe52525febc1a376566af9bf4d2d75da6e2767`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 4/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T04:23:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33 | 0 | 1 | 1 | 290 | 0 |  |
| model-binding:groq-llama33-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T04:23:35Z |
| model-provider:groq | 0 | 1 | 1 | 290 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
