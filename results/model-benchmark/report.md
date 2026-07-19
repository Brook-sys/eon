# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `offline-oracle`
- Runs: 66
- Compiled: 66
- Syntax valid: 66
- Semantically correct: 66
- Input tokens: 2112
- Output tokens: 210
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `offline-oracle`
- Verdict: `PASS`
- Headline: Offline oracle PASS on "cognitive-v2": 66/66 runs semantically correct (encode→Parse ceiling; not a live model skill).

### Notes

- `interpret:compiled=66`
- `interpret:encode_parse_roundtrip_ok`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=offline-oracle`
- `interpret:model=offline-oracle`
- `interpret:oracle_is_harness_ceiling_not_model_skill`
- `interpret:semantically_correct=66`
- `interpret:strongest_format=CHOICE rate=18/18`
- `interpret:syntax_valid=66`
- `interpret:total=66`
- `interpret:verdict=PASS`
- `interpret:weakest_context=2048 rate=22/22`
- `interpret:weakest_format=CHOICE rate=18/18`
- `interpret:weakest_operation=CONFLICT rate=18/18`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 18 | 18 | 18 | 18 | 0 | 0 |
| EXTRACT | 18 | 18 | 18 | 18 | 0 | 0 |
| REPAIR | 12 | 12 | 12 | 12 | 0 | 0 |
| SYNTHESIZE | 18 | 18 | 18 | 18 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 18 | 18 | 18 | 18 | 0 | 0 |
| DELIMITED | 24 | 24 | 24 | 24 | 0 | 0 |
| JSON | 24 | 24 | 24 | 24 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 22 | 22 | 0 | 0 |
| 4096 | 22 | 22 | 22 | 22 | 0 | 0 |
| 8192 | 22 | 22 | 22 | 22 | 0 | 0 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-date | EXTRACT | CHOICE | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| extract-date | EXTRACT | JSON | 4096 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| extract-date | EXTRACT | JSON | 8192 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-qualified-date | EXTRACT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-qualified-date | EXTRACT | CHOICE | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| extract-qualified-date | EXTRACT | JSON | 4096 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| extract-qualified-date | EXTRACT | JSON | 8192 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 4096 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 8192 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| detect-conflict | CONFLICT | JSON | 4096 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| detect-conflict | CONFLICT | JSON | 8192 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| distinguish-temporal-change | CONFLICT | JSON | 4096 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| distinguish-temporal-change | CONFLICT | JSON | 8192 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 32/9 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 32/9 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 32/9 | 0 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 32/8 | 0 ms |
| repair-anchor | REPAIR | JSON | 4096 | CORRECT | 0 | 0 ms | 32/8 | 0 ms |
| repair-anchor | REPAIR | JSON | 8192 | CORRECT | 0 | 0 ms | 32/8 | 0 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 32/3 | 0 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| repair-unrecoverable | REPAIR | JSON | 4096 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| repair-unrecoverable | REPAIR | JSON | 8192 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
