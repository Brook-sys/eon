# Malformed recovery campaign

- Name: `phase128-malformed-recovery-groq-short-correction`
- Model calls: 2
- Injected malformed calls: 1
- External live calls: 1
- Recovery stages: `SHORT_CORRECTION`
- Commit: `commit_0000000000000006`
- Operation state: `SUCCEEDED`
- Completion receipts: 2
- Canonical entity stored: `true`
- Durable reopen verified: `true`

| Call | Binding | Source | Success | Latency | Input tokens | Output tokens | Finish | Bytes | SHA-256 | Error |
| ---: | --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- | --- |
| 1 | groq-llama-fallback | deterministic malformed | true | 0s | 0 | 0 | stop | 179 | d3af5198be70deaf0cf1264113b070fe149418bbc4af0e6fe652e7e1d871e179 |  |
| 2 | groq-llama-fallback | live | true | 740.972506ms | 167 | 214 | stop | 180 | 0305a55d2c55006dd9b7db5c52ab7c382f934f81131d3fae543aaff0b5c2c0c9 |  |

Provider text is not stored in this report. Injected outputs are known non-effects; the live output remains subject to typed parsing, validators, commit authority, and receipt hashing.
