# Runtime provider gate campaign

- Name: `phase206-qwen36-minimal-json-control`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-mistral-small-4-seeded`
- Selected route: `groq` / `groq-qwen36-27b`
- Provider success: `true`
- Provider latency: `554.004527ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 223
- Response SHA-256: `1b5fc5b4806b422124971f6ab460942b6f083cc4148a4afa8050dd1b4e5158e8`
- Expected response configured: `true`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: `expected_with_prefix_and_suffix`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T04:12:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen36-27b | 0 | 1 | 1 | 154 | 0 |  |
| model-binding:nim-mistral-small-4-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-25T04:12:16Z |
| model-provider:groq | 0 | 1 | 1 | 154 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
