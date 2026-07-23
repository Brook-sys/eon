# Malformed recovery campaign

- Name: `simpler-format-recovery-campaign-proxy-reject`
- Model calls: 3
- Injected malformed calls: 3
- External live calls: 0
- Recovery stages: `SHORT_CORRECTION -> SIMPLER_FORMAT -> DEFER`
- Commit: ``
- Operation state: `EXHAUSTED`
- Completion receipts: 3
- Canonical entity stored: `false`
- Durable reopen verified: `true`

| Call | Binding | Source | Success | Latency | Input tokens | Output tokens | Finish | Bytes | SHA-256 | Error |
| ---: | --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- | --- |
| 1 | nim | deterministic malformed | true | 0s | 0 | 0 | stop | 175 | 5600250e01cbbb32313ea543b465b96f187d921421143b007aff9196b3ad1a74 |  |
| 2 | nim | deterministic malformed | true | 0s | 0 | 0 | stop | 554 | ef3627b02bad3d4a82d599eed406a16c3ba9b1ff11020ba37d6adbf5c9e1d557 |  |
| 3 | nim | deterministic malformed | true | 0s | 0 | 0 | stop | 554 | ef3627b02bad3d4a82d599eed406a16c3ba9b1ff11020ba37d6adbf5c9e1d557 |  |

Provider text is not stored in this report. Injected outputs are known non-effects; the live output remains subject to typed parsing, validators, commit authority, and receipt hashing.
