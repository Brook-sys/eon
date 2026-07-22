# Runtime provider gate campaign

- Name: `nim-primary-circuit-to-groq-fallback-2026-07-22-1140`
- External calls: 1/1
- Seeded circuit: `model-binding:nim-primary`
- Selected route: `groq` / `groq-fallback`
- Provider success: `false`
- Provider error class: `http`
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Second acquire: `model_route_unavailable`
- Operation state after local throttle: `READY`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-fallback | 0 | 1 | 1 | 32 | 1 | 2026-07-22T14:46:52Z |
| model-binding:nim-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-22T14:46:51Z |
| model-provider:groq | 0 | 1 | 1 | 32 | 0 |  |
| model-provider:nvidia-nim | 0 | 0 | 0 | 0 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
