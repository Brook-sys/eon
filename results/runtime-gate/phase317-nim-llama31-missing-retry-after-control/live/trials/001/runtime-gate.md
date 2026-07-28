# Runtime provider gate campaign

- Name: `phase317-nim-llama31-missing-retry-after-control`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-llama31-70b-seeded`
- Selected route: `nim` / `nim-llama31-70b`
- Provider success: `true`
- Provider latency: `11.029754672s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 131
- Response SHA-256: `2280d3ac5f4cbb4d7d48a28744b058584afc2dbfba7998e984b559a152a2e8ca`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T08:44:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:nim-llama31-70b | 0 | 1 | 1 | 239 | 0 |  |
| model-binding:nim-llama31-70b-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T08:44:01Z |
| model-provider:nim | 0 | 1 | 1 | 239 | 0 |  |
| model-provider:nim-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
