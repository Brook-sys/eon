# Runtime provider gate campaign

- Name: `phase277-groq-llama33-incomplete-evidence-control`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-seeded-primary`
- Selected route: `groq` / `groq-llama33-70b`
- Provider success: `true`
- Provider latency: `325.216102ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 117
- Response SHA-256: `4dfff3f894f667bf79d4e58ffe9d18f637441425727c75f1994976d9d15adca8`
- Expected response configured: `true`
- Expected response exact match: `false`
- Structural comparison configured: `true`
- Structural overall match: `true`
- Structural fields matched/mismatched/absent: 4/0/0
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-27T19:03:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama33-70b | 0 | 1 | 1 | 221 | 0 |  |
| model-binding:groq-seeded-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-27T19:03:37Z |
| model-provider:groq | 0 | 1 | 1 | 221 | 0 |  |
| model-provider:groq-seeded | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
