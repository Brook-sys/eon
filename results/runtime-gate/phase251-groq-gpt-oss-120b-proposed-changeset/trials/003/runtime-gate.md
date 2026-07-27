# Runtime provider gate campaign

- Name: `phase251-groq-gpt-oss-120b-proposed-changeset`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded`
- Selected route: `groq` / `groq-gpt-oss-120b`
- Provider success: `true`
- Provider latency: `1.099644197s`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 244
- Response SHA-256: `130ad249e01737f2fd50b512f40342033bfa12927d8bc6c3a8a48811859a238c`
- Expected response configured: `false`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: `invalid_json`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-gpt-oss-120b | 0 | 1 | 1 | 1063 | 0 |  |
| model-binding:nim-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T09:03:42Z |
| model-provider:groq | 0 | 1 | 1 | 1063 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
