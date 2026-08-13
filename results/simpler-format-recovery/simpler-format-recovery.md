# Malformed recovery campaign

- Name: `simpler_format_exhaustion`
- Model calls: 3
- Injected malformed calls: 2
- External live calls: 1
- Recovery stages: `SHORT_CORRECTION -> SIMPLER_FORMAT`
- Commit: `commit_0000000000000006`
- Operation state: `SUCCEEDED`
- Completion receipts: 3
- Canonical entity stored: `true`
- Durable reopen verified: `true`

| Call | Binding | Source | Success | Latency | Input tokens | Output tokens | Finish | Bytes | SHA-256 | Error |
| ---: | --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- | --- |
| 1 | groq-llama8b | deterministic malformed | true | 0s | 0 | 0 | stop | 175 | 5600250e01cbbb32313ea543b465b96f187d921421143b007aff9196b3ad1a74 |  |
| 2 | groq-llama8b | deterministic malformed | true | 0s | 0 | 0 | stop | 554 | ef3627b02bad3d4a82d599eed406a16c3ba9b1ff11020ba37d6adbf5c9e1d557 |  |
| 3 | groq-llama8b | live | true | 556.752156ms | 432 | 143 | stop | 534 | 3302c6a5af280c4b980739b9cb4354eadfab103a3c89e5ef1d552110f509db7e |  |

Provider text is not stored in this report. Injected outputs are known non-effects; the live output remains subject to typed parsing, validators, commit authority, and receipt hashing.
