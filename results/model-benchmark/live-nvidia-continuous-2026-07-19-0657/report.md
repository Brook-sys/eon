# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `mistralai/mistral-small-4-119b-2603`
- Runs: 22
- Compiled: 22
- Syntax valid: 20
- Semantically correct: 20
- Input tokens: 3894
- Output tokens: 433
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/2
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "mistralai/mistral-small-4-119b-2603" on "cognitive-v2": correct=20/22 syntax_valid=20 (provider_errors=0 validation_errors=2).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=2`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=mistralai/mistral-small-4-119b-2603`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=20`
- `interpret:strongest_format=JSON rate=8/8`
- `interpret:syntax_valid=20`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=20/22`
- `interpret:weakest_format=CHOICE rate=5/6`
- `interpret:weakest_operation=REPAIR rate=3/4`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 5 | 5 | 0 | 1 |
| EXTRACT | 6 | 6 | 6 | 6 | 0 | 0 |
| REPAIR | 4 | 4 | 3 | 3 | 0 | 1 |
| SYNTHESIZE | 6 | 6 | 6 | 6 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 5 | 5 | 0 | 1 |
| DELIMITED | 8 | 8 | 7 | 7 | 0 | 1 |
| JSON | 8 | 8 | 8 | 8 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 20 | 20 | 0 | 2 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 170/20 | 850 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 168/20 | 737 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 166/33 | 715 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 228/20 | 641 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 226/20 | 523 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 224/33 | 705 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 145/11 | 440 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 143/11 | 476 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 141/24 | 570 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 185/12 | 462 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 183/12 | 466 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 181/25 | 687 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 179/7 | 419 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 177/14 | 577 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 175/27 | 587 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 189/17 | 519 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 187/14 | 499 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 185/27 | 588 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 150/17 | 497 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 148/30 | 665 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | VALIDATION | 0 | 0 ms | 173/13 | 755 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 171/26 | 569 ms |
