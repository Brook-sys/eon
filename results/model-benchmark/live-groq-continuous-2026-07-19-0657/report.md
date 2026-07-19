# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `llama-3.1-8b-instant`
- Runs: 22
- Compiled: 22
- Syntax valid: 13
- Semantically correct: 12
- Input tokens: 3206
- Output tokens: 1470
- Omitted facts: 0
- Errors (compile/provider/validation): 0/5/4
- Rate limited / timed out: 5/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.1-8b-instant" on "cognitive-v2": correct=12/22 syntax_valid=13 (provider_errors=5 validation_errors=4).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=5`
- `interpret:errors_validation=4`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.1-8b-instant`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=12`
- `interpret:strongest_format=CHOICE rate=6/6`
- `interpret:syntax_valid=13`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=12/22`
- `interpret:weakest_format=JSON rate=1/8`
- `interpret:weakest_operation=REPAIR rate=0/4`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 4 | 4 | 0 | 2 |
| EXTRACT | 6 | 6 | 4 | 4 | 0 | 2 |
| REPAIR | 4 | 4 | 0 | 0 | 0 | 4 |
| SYNTHESIZE | 6 | 6 | 5 | 4 | 0 | 1 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 6 | 6 | 0 | 0 |
| DELIMITED | 8 | 8 | 6 | 5 | 0 | 2 |
| JSON | 8 | 8 | 1 | 1 | 0 | 7 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 13 | 12 | 0 | 9 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 175/46 | 452 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 173/14 | 305 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 172/256 | 484 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 220/26 | 326 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 218/14 | 250 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 217/256 | 575 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 161/9 | 263 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 159/9 | 308 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 0 | 0 ms | 158/256 | 624 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 201/10 | 243 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 199/8 | 323 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 198/256 | 723 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 188/13 | 218 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 186/13 | 224 ms |
| detect-conflict | CONFLICT | JSON | 2048 | VALIDATION | 0 | 0 ms | 185/256 | 665 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 199/14 | 236 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 197/14 | 231 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 50 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 3000 ms | 0/0 | 51 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 429 | 3000 ms | 0/0 | 50 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 3000 ms | 0/0 | 48 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 47 ms |
