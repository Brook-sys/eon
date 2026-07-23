# Fallback-model recovery campaign

- Name: `phase130-fallback-model-recovery-nim-mistral-small-4`
- Model calls: 4
- Injected validation non-effects: 3
- External live calls: 1
- Recovery stages: `SHORT_CORRECTION -> SIMPLER_FORMAT -> FALLBACK_MODEL`
- Binding switch: `groq-primary` -> `nim-fallback` (verified: `true`)
- Commit: `commit_0000000000000007`
- Operation state: `SUCCEEDED`
- Completion receipts: 4
- Canonical entity stored: `true`
- Durable reopen verified: `true`

| Call | Binding | Source | Success | Latency | Input tokens | Output tokens | Finish | Bytes | SHA-256 | Error |
| ---: | --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- | --- |
| 1 | groq-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 177 | 401097b462fce341f808e0c0c17bf011b3157a443ce15019f216300d0715f439 |  |
| 2 | groq-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 559 | 8079c00634aeee9b318fb06a576c44181a04950cf076e8f0302ccef63c5308c2 |  |
| 3 | groq-primary | deterministic malformed | true | 0s | 0 | 0 | stop | 166 | 5affad6a136b8ae541b0888e89cd27c0d93ea1580ab0c8c7e259d71a3989f7d5 |  |
| 4 | nim-fallback | live | true | 2.980244418s | 612 | 151 | stop | 538 | c8b890b7404dd80e60cb52033da18f4b52b504723bc698cf55353aef53e4625c |  |

Provider text is not stored in this report. Injected outputs are known non-effects; the live output remains subject to typed parsing, validators, commit authority, and receipt hashing.
