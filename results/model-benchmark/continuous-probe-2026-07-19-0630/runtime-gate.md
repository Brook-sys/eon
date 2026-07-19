# Runtime provider gate campaign

- Name: `continuous-probe-2026-07-19-nim`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-primary`
- Selected route: `nim` / `nim-fallback`
- Provider success: `true`
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Second acquire: `resource_resource_rate_limit` until `2026-07-19T03:08:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-19T03:08:24Z |
| model-binding:nim-fallback | 0 | 1 | 1 | 278 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nim | 0 | 1 | 1 | 278 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
