# Defer-replan recovery campaign

- Name: `phase132-defer-replan-nim-mistral-small-4`
- Model calls: 4
- Preparatory injected non-effects: 3
- External live calls: 1
- Recovery stages: `SHORT_CORRECTION -> SIMPLER_FORMAT -> FALLBACK_MODEL -> DEFER`
- Binding switch: `groq-primary` -> `nim-fallback` (verified: `true`)
- Final decision: `REPLAN` (`intra_execute_recovery_exhausted_replan_allowed`)
- Operation state/attempt: `READY` / 1
- Replan/model-failed/exhaustion events: 1 / 1 / 0
- Completion receipts: 4
- Canonical entity absent: `true`
- Durable reopen verified: `true`

| Call | Binding | Source | Success | Latency | Input tokens | Output tokens | Finish | Live bytes | Live SHA-256 | Presented invalid | Receipt bytes | Receipt SHA-256 | Error |
| ---: | --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- | --- | ---: | --- | --- |
| 1 | groq-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 167 | ab8ca932bfc428c765a4eee39c973f0870dee3965c107063dc038d7d9209c67b | false | 0 |  |  |
| 2 | groq-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 532 | 33386b62373d173b66a6973eca748876db106b21b29b8ba0a773c80f4bddc3b4 | false | 0 |  |  |
| 3 | groq-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 146 | 51b05d216b02bff55874e2359f7eb03a928328bba5692fad2fe3bedd77a318fe | false | 0 |  |  |
| 4 | nim-fallback | live | true | 2.173078152s | 614 | 142 | stop | 477 | 6e2336d6466c534e486c1338d10886e1e051d0836ea624439536f0d534741dbe | true | 146 | 51b05d216b02bff55874e2359f7eb03a928328bba5692fad2fe3bedd77a318fe |  |

Provider text is not stored. The fourth call records live metrics/hash separately from the deterministic invalid non-effect presented to the executor and persisted in its completion receipt.
