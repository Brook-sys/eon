# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `offline-oracle`
- Runs: 33
- Compiled: 33
- Syntax valid: 33
- Semantically correct: 33
- Input tokens: 1056
- Output tokens: 147
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/0

## Interpretation

- Kind: `offline-oracle`
- Verdict: `PASS`
- Headline: Offline oracle PASS on "cognitive-v1": 33/33 runs semantically correct (encode→Parse ceiling; not a live model skill).

### Notes

- `interpret:compiled=33`
- `interpret:encode_parse_roundtrip_ok`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=offline-oracle`
- `interpret:model=offline-oracle`
- `interpret:oracle_is_harness_ceiling_not_model_skill`
- `interpret:semantically_correct=33`
- `interpret:syntax_valid=33`
- `interpret:total=33`
- `interpret:verdict=PASS`
- `interpret:weakest_context=2048 rate=11/11`
- `interpret:weakest_format=CHOICE rate=9/9`
- `interpret:weakest_operation=CONFLICT rate=9/9`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 9 | 9 | 0 | 0 |
| EXTRACT | 9 | 9 | 9 | 9 | 0 | 0 |
| REPAIR | 6 | 6 | 6 | 6 | 0 | 0 |
| SYNTHESIZE | 9 | 9 | 9 | 9 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 9 | 9 | 0 | 0 |
| DELIMITED | 12 | 12 | 12 | 12 | 0 | 0 |
| JSON | 12 | 12 | 12 | 12 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 11 | 11 | 0 | 0 |
| 4096 | 11 | 11 | 11 | 11 | 0 | 0 |
| 8192 | 11 | 11 | 11 | 11 | 0 | 0 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 32/3 | 0 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 32/3 | 0 ms |
| extract-date | EXTRACT | CHOICE | 8192 | CORRECT | 32/3 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 32/5 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 32/5 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 32/5 | 0 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 32/2 | 0 ms |
| extract-date | EXTRACT | JSON | 4096 | CORRECT | 32/2 | 0 ms |
| extract-date | EXTRACT | JSON | 8192 | CORRECT | 32/2 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 32/3 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 32/5 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 32/5 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 32/5 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 32/2 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | CORRECT | 32/2 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | CORRECT | 32/2 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | CORRECT | 32/3 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | CORRECT | 32/3 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | CORRECT | 32/3 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 32/5 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | CORRECT | 32/5 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | CORRECT | 32/5 | 0 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 32/2 | 0 ms |
| detect-conflict | CONFLICT | JSON | 4096 | CORRECT | 32/2 | 0 ms |
| detect-conflict | CONFLICT | JSON | 8192 | CORRECT | 32/2 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 32/11 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | CORRECT | 32/11 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | CORRECT | 32/11 | 0 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 32/8 | 0 ms |
| repair-anchor | REPAIR | JSON | 4096 | CORRECT | 32/8 | 0 ms |
| repair-anchor | REPAIR | JSON | 8192 | CORRECT | 32/8 | 0 ms |
