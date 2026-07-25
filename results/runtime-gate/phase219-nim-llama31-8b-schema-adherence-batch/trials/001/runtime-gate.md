# Runtime provider gate campaign

- Name: `phase219-nim-llama31-8b-schema-adherence-batch`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-70b-seeded`
- Selected route: `nim` / `nim-meta-llama31-8b`
- Provider success: `true`
- Provider latency: `1.523516681s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 480
- Response SHA-256: `e32afe7c591f45cfc8f5ccdac6e9d9d00b427bd60581416416dda578f641c879`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T15:03:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T15:03:49Z |
| model-binding:nim-meta-llama31-8b | 0 | 1 | 1 | 719 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 719 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
