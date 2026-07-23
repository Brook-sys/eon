# Runtime provider gate campaign

- Name: `phase128-control-single-call-nim`
- External calls: 1/1
- Seeded circuit: `model-binding:groq-llama-primary`
- Selected route: `nvidia` / `nvidia-mistral-fallback`
- Provider success: `true`
- Provider latency: `1.091236583s`
- Provider error class: ``
- Provider HTTP status: 0
- Provider Retry-After: `0s`
- Finish reason: `stop`
- Response bytes: 43
- Response SHA-256: `92400f00b6635a2f250e27918b1bc5e4f6bd2ef35a313ce85ec27c1276819256`
- Expected response configured: `true`
- Expected response exact match: `false`
- Response JSON valid: `true`
- Response framing class: `valid_json_mismatch`
- Second acquire: `resource_resource_rate_limit` until `2026-07-23T02:43:00Z`
- Operation state after local throttle: `WAITING_TIME`
- Durable reopen verified: `true`

| Resource | In flight | Calls/min | Calls/day | Tokens/min | Failures | Circuit open until |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| model-binding:groq-llama-primary | 0 | 0 | 0 | 0 | 1 | 2026-07-23T02:43:08Z |
| model-binding:nvidia-mistral-fallback | 0 | 1 | 1 | 139 | 0 |  |
| model-provider:groq | 0 | 0 | 0 | 0 | 0 |  |
| model-provider:nvidia | 0 | 1 | 1 | 139 | 0 |  |

The primary circuit and one-call minute quota were seeded control state. No 429 was intentionally induced; any HTTP status or Retry-After above was naturally observed from the single useful provider call. This report has no authority to change runtime bindings.
