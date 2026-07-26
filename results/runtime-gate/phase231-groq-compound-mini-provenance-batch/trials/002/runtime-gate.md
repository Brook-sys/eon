# Runtime provider gate campaign

- Name: `phase231-groq-compound-mini-provenance`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-mistral-small-4-seeded`
- Selected route: `groq` / `groq-compound-mini`
- Provider success: `true`
- Provider latency: `2.95383581s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 498
- Response SHA-256: `3141c50b0e5e62b6565ae9f1fe345a17fb069b93195aafdfd813e63991da1c78`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-26T23:27:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-compound-mini | 0 | 1 | 1 | 2708 | 0 |  |
| model-binding:nim-mistral-small-4-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-26T23:27:24Z |
| model-provider:groq | 0 | 1 | 1 | 2708 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
