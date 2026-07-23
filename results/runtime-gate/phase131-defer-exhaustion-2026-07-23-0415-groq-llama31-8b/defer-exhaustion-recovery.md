# Defer-exhaustion recovery campaign

- Name: `phase131-defer-exhaustion-groq-llama31-8b`
- Model calls: 4
- Preparatory injected non-effects: 3
- External live calls: 1
- Recovery stages: `SHORT_CORRECTION -> SIMPLER_FORMAT -> FALLBACK_MODEL -> DEFER`
- Binding switch: `nim-primary` -> `groq-fallback` (verified: `true`)
- Final decision: `EXHAUST` (`model_recovery_budget_exhausted`)
- Operation state: `EXHAUSTED`
- Completion receipts: 4
- Canonical entity absent: `true`
- Durable reopen verified: `true`

| Call | Binding | Source | Success | Latency | Input tokens | Output tokens | Finish | Live bytes | Live SHA-256 | Presented invalid | Receipt bytes | Receipt SHA-256 | Error |
| ---: | --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- | --- | ---: | --- | --- |
| 1 | nim-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 175 | 7239eb1a18de2bb99ca4ea6629768a8c98af12c3c7c9e394f1ac1b91fbaac49e | false | 0 |  |  |
| 2 | nim-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 556 | 2125b65e9a544a3f5422e19d7bbf13b34ef054f373ccc2fa6a4845836a4f373b | false | 0 |  |  |
| 3 | nim-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 162 | 6ac8f295a79dbd20e9fddc95a8d025c459dd09ce896ad80c9e287b937e2c4625 | false | 0 |  |  |
| 4 | groq-fallback | live | true | 500.728138ms | 625 | 168 | stop | 585 | d10c5261d5a01d0616f6d37a250e57301b9944c31ce9a48f8d5530d5cf5ec811 | true | 162 | 6ac8f295a79dbd20e9fddc95a8d025c459dd09ce896ad80c9e287b937e2c4625 |  |

Provider text is not stored. The fourth call records live metrics/hash separately from the deterministic invalid non-effect presented to the executor and persisted in its completion receipt.
