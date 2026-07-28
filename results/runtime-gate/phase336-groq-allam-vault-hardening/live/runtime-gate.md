# Runtime provider gate campaign

- Name: `phase336-groq-allam-vault-hardening`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama33-seeded`
- Selected route: `groq` / `groq-allam`
- Provider success: `true`
- Provider latency: `470.175296ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 159
- Response SHA-256: `54f391e16f244951339f05addaddd442b0cbffce57b8e18969c3d1531afe2ca6`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 5/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-28T22:46:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-allam | 0 | 1 | 1 | 303 | 0 |  |
| model-binding:groq-llama33-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-28T22:46:05Z |
| model-provider:groq | 0 | 1 | 1 | 303 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
