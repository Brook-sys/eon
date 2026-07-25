# Runtime provider gate campaign

- Name: `phase218a-nim-llama31-70b-schema-adherence`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-70b-seeded`
- Selected route: `nim` / `nim-meta-llama31-70b`
- Provider success: `true`
- Provider latency: `8.85330722s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 573
- Response SHA-256: `62c629f2d41d0c7f6bc58c521c2b88802c37c4f571743647b7f052394acdf5da`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T11:43:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T11:43:38Z |
| model-binding:nim-meta-llama31-70b | 0 | 1 | 1 | 758 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 758 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
