# Runtime provider gate campaign

- Name: `phase445-parser-resilience`
- External calls: 1/1
- Seeded circuit: `model-binding:b2`
- Selected route: `nim-deepseek` / `b1`
- Provider success: `true`
- Provider latency: `1.342437046s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 41
- Response SHA-256: `393b4e5c3db492f50dd5daf2fcf8f86fba1f43cbd9734d8505024f9dcbef416d`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `false`
- Structural overall match: `false`
- Structural fields matched/mismatched/absent: 0/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Budget retry attempted: `false`
- Budget retry output tokens: 0
- Second acquire: `resource_resource_rate_limit` until `2026-08-09T19:10:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:b1 | 0 | 1 | 1 | 142 | 0 |  |
| model-binding:b2 | 0 | 0 | 0 | 0 | 1 | 2026-08-09T19:10:04Z |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim-deepseek | 0 | 1 | 1 | 142 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
