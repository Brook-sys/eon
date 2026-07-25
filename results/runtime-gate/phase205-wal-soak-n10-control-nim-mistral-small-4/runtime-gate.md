# Runtime provider gate campaign

- Name: `phase205-wal-soak-n10-control-nim-mistral-small-4`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-open`
- Selected route: `nim` / `nim-mistral-small-4`
- Provider success: `true`
- Provider latency: `803.367567ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 21
- Response SHA-256: `40f09d3a9ef7bd003afa690fd3593734944913c207610f6220a4aaf690608041`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-25T03:26:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-seeded-open | 0 | 0 | 0 | 0 | 1 | 2026-07-25T03:25:46Z |
| model-binding:nim-mistral-small-4 | 0 | 1 | 1 | 128 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 128 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
