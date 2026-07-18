# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `mistralai/mistral-small-4-119b-2603`
- Runs: 11
- Compiled: 11
- Syntax valid: 9
- Semantically correct: 9
- Input tokens: 1762
- Output tokens: 205
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/2
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "mistralai/mistral-small-4-119b-2603" on "cognitive-v1": correct=9/11 syntax_valid=9 (provider_errors=0 validation_errors=2).

### Notes

- `interpret:compiled=11`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=2`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=mistralai/mistral-small-4-119b-2603`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=9`
- `interpret:strongest_format=JSON rate=4/4`
- `interpret:syntax_valid=9`
- `interpret:total=11`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=9/11`
- `interpret:weakest_format=CHOICE rate=2/3`
- `interpret:weakest_operation=CONFLICT rate=1/3`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 3 | 3 | 1 | 1 | 0 | 2 |
| EXTRACT | 3 | 3 | 3 | 3 | 0 | 0 |
| REPAIR | 2 | 2 | 2 | 2 | 0 | 0 |
| SYNTHESIZE | 3 | 3 | 3 | 3 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 3 | 3 | 2 | 2 | 0 | 1 |
| DELIMITED | 4 | 4 | 3 | 3 | 0 | 1 |
| JSON | 4 | 4 | 4 | 4 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 9 | 9 | 0 | 2 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 170/20 | 767 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 168/20 | 497 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 166/33 | 592 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 145/11 | 433 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 143/11 | 409 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 141/24 | 558 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 179/7 | 400 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | VALIDATION | 0 | 0 ms | 177/5 | 380 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 175/27 | 2987 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 150/17 | 542 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 148/30 | 595 ms |
