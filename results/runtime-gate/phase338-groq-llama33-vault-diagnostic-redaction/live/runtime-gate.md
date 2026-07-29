# Runtime provider gate campaign

- Name: `phase338-groq-llama33-vault-diagnostic-redaction`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-compound-seeded`
- Selected route: `groq` / `groq-llama33`
- Provider success: `true`
- Provider latency: `513.094182ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 169
- Response SHA-256: `4b36a1302a044a6af4eabb08b92340b4814b235c72f219c89d63ae1351eff7db`
- Expected response configured: `true`
- Expected response exact match: `true`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 6/0/0
- Response JSON valid: `true`
- Response framing class: `exact`
- Second acquire: `resource_resource_rate_limit` until `2026-07-29T00:44:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-compound-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-29T00:44:02Z |
| model-binding:groq-llama33 | 0 | 1 | 1 | 249 | 0 |  |
| model-provider:groq | 0 | 1 | 1 | 249 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
