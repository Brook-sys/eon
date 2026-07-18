# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `llama-3.1-8b-instant`
- Runs: 33
- Compiled: 33
- Syntax valid: 12
- Semantically correct: 12
- Input tokens: 3744
- Output tokens: 1828
- Omitted facts: 0
- Errors (compile/provider/validation): 0/11/10

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.1-8b-instant" on "cognitive-v1": correct=12/33 syntax_valid=12 (provider_errors=11 validation_errors=10).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=11`
- `interpret:errors_validation=10`
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
- `interpret:weakest_operation=REPAIR rate=0/6`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 3 | 3 | 0 | 6 |
| EXTRACT | 9 | 9 | 6 | 6 | 0 | 3 |
| REPAIR | 6 | 6 | 0 | 0 | 0 | 6 |
| SYNTHESIZE | 9 | 9 | 3 | 3 | 0 | 6 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 9 | 9 | 0 | 0 |
| DELIMITED | 12 | 12 | 3 | 3 | 0 | 9 |
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
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 175/46 | 398 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 175/46 | 514 ms |
| extract-date | EXTRACT | CHOICE | 8192 | CORRECT | 175/46 | 357 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 173/16 | 219 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 173/16 | 226 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 173/16 | 264 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 172/256 | 450 ms |
| extract-date | EXTRACT | JSON | 4096 | VALIDATION | 172/256 | 472 ms |
| extract-date | EXTRACT | JSON | 8192 | VALIDATION | 172/256 | 479 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 161/9 | 299 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 161/9 | 206 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 161/9 | 295 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | VALIDATION | 159/9 | 291 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | VALIDATION | 159/9 | 204 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | VALIDATION | 159/9 | 221 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 158/256 | 603 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | VALIDATION | 158/256 | 526 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | VALIDATION | 158/256 | 560 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | CORRECT | 188/13 | 231 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | CORRECT | 188/13 | 225 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | CORRECT | 188/13 | 225 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | VALIDATION | 186/13 | 219 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | PROVIDER | 0/0 | 34 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | PROVIDER | 0/0 | 37 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 0/0 | 37 ms |
| detect-conflict | CONFLICT | JSON | 4096 | PROVIDER | 0/0 | 35 ms |
| detect-conflict | CONFLICT | JSON | 8192 | PROVIDER | 0/0 | 36 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 0/0 | 37 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | PROVIDER | 0/0 | 39 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | PROVIDER | 0/0 | 41 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 0/0 | 44 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 0/0 | 47 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 0/0 | 46 ms |
