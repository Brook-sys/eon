# Runtime provider gate campaign

- Name: `phase152-terminal-election-release-control-nim-llama31-8b`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-unused`
- Selected route: `nim` / `nim-llama31-8b`
- Provider success: `true`
- Provider latency: `735.576489ms`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 34
- Response SHA-256: `5271f5e48d65a065f664a6761accad53460834ad4682034394981a35e3f0ec8f`
- Expected response configured: `true`
- Expected response exact match: `true`
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-23T13:24:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-unused | 0 | 0 | 0 | 0 | 1 | 2026-07-23T13:23:17Z |
| model-binding:nim-llama31-8b | 0 | 1 | 1 | 130 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 130 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
