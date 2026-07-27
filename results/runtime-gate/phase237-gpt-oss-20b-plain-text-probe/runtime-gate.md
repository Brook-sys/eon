# Runtime provider gate campaign

- Name: `phase237-gpt-oss-20b-plain-text-probe`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded-open`
- Selected route: `groq` / `groq-gpt-oss-20b`
- Provider success: `false`
- Provider latency: `402.020393ms`
- Provider error class: `provider`
- Provider error reason: `empty_content`
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: ``
- Response bytes: 0
- Response SHA-256: ``
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: ``
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-gpt-oss-20b | 0 | 1 | 1 | 64 | 1 | 2026-07-27T01:49:11Z |
| model-binding:nim-seeded-open | 0 | 0 | 0 | 0 | 1 | 2026-07-27T01:48:11Z |
| model-provider:groq | 0 | 1 | 1 | 64 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
