# Runtime provider gate campaign

- Name: `phase249-groq-qwen36-27b-exact-json-128`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-seeded`
- Selected route: `groq` / `groq-qwen36-27b`
- Provider success: `true`
- Provider latency: `598.012529ms`
- Provider error class: ``
- Provider error reason: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `length`
- Response bytes: 524
- Response SHA-256: `38a9c4be0665996eb8276916871cf3dd6072448c78449771957c1ac512161efa`
- Expected response configured: `true`
- Expected response exact match: `false`
- Response JSON valid: `false`
- Response framing class: `expected_with_prefix_and_suffix`
- Second acquire: ``
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-qwen36-27b | 0 | 1 | 1 | 226 | 0 |  |
| model-binding:nim-seeded | 0 | 0 | 0 | 0 | 1 | 2026-07-27T07:43:26Z |
| model-provider:groq | 0 | 1 | 1 | 226 | 0 |  |
| model-provider:nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
