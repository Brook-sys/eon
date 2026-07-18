# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `llama-3.1-8b-instant`
- Runs: 33
- Compiled: 33
- Syntax valid: 12
- Semantically correct: 12
- Input tokens: 2520
- Output tokens: 1002
- Omitted facts: 0
- Errors (compile/provider/validation): 0/18/3

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.1-8b-instant" on "cognitive-v1": correct=12/33 syntax_valid=12 (provider_errors=18 validation_errors=3).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=18`
- `interpret:errors_validation=3`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.1-8b-instant`
- `interpret:prefer_weaker_formats_or_smaller_ops_first`
- `interpret:semantically_correct=12`
- `interpret:syntax_valid=12`
- `interpret:total=33`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=4/11`
- `interpret:weakest_format=JSON rate=0/12`
- `interpret:weakest_operation=CONFLICT rate=0/9`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 0 | 0 | 0 | 9 |
| EXTRACT | 9 | 9 | 6 | 6 | 0 | 3 |
| REPAIR | 6 | 6 | 0 | 0 | 0 | 6 |
| SYNTHESIZE | 9 | 9 | 6 | 6 | 0 | 3 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 6 | 6 | 0 | 3 |
| DELIMITED | 12 | 12 | 6 | 6 | 0 | 6 |
| JSON | 12 | 12 | 0 | 0 | 0 | 12 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 4 | 4 | 0 | 7 |
| 4096 | 11 | 11 | 4 | 4 | 0 | 7 |
| 8192 | 11 | 11 | 4 | 4 | 0 | 7 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 175/46 | 432 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 175/46 | 354 ms |
| extract-date | EXTRACT | CHOICE | 8192 | CORRECT | 175/46 | 273 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 173/14 | 214 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 173/14 | 253 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 173/14 | 294 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 172/256 | 471 ms |
| extract-date | EXTRACT | JSON | 4096 | VALIDATION | 172/256 | 534 ms |
| extract-date | EXTRACT | JSON | 8192 | VALIDATION | 172/256 | 459 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 161/9 | 213 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 161/9 | 292 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 161/9 | 287 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 159/9 | 207 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 159/9 | 202 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 159/9 | 291 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | PROVIDER | 0/0 | 31 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | PROVIDER | 0/0 | 35 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | PROVIDER | 0/0 | 31 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 0/0 | 35 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | PROVIDER | 0/0 | 32 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | PROVIDER | 0/0 | 33 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 0/0 | 30 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | PROVIDER | 0/0 | 35 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | PROVIDER | 0/0 | 34 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 0/0 | 34 ms |
| detect-conflict | CONFLICT | JSON | 4096 | PROVIDER | 0/0 | 30 ms |
| detect-conflict | CONFLICT | JSON | 8192 | PROVIDER | 0/0 | 33 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 0/0 | 43 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | PROVIDER | 0/0 | 34 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | PROVIDER | 0/0 | 35 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 0/0 | 34 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 0/0 | 32 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 0/0 | 35 ms |
