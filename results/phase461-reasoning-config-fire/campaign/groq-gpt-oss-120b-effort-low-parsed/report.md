# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `openai/gpt-oss-120b`
- Runs: 22
- Compiled: 22
- Syntax valid: 13
- Semantically correct: 13
- Input tokens: 2912
- Output tokens: 2506
- Omitted facts: 0
- Errors (compile/provider/validation): 0/9/0
- Rate limited / timed out: 9/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "openai/gpt-oss-120b" on "cognitive-v2": correct=13/22 syntax_valid=13 (provider_errors=9 validation_errors=0).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=9`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=openai/gpt-oss-120b`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=13`
- `interpret:strongest_format=CHOICE rate=4/6`
- `interpret:syntax_valid=13`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=13/22`
- `interpret:weakest_format=JSON rate=4/8`
- `interpret:weakest_operation=REPAIR rate=0/4`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 1 | 1 | 0 | 5 |
| EXTRACT | 6 | 6 | 6 | 6 | 0 | 0 |
| REPAIR | 4 | 4 | 0 | 0 | 0 | 4 |
| SYNTHESIZE | 6 | 6 | 6 | 6 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 4 | 4 | 0 | 2 |
| DELIMITED | 8 | 8 | 5 | 5 | 0 | 3 |
| JSON | 8 | 8 | 4 | 4 | 0 | 4 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 13 | 13 | 0 | 9 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 211/110 | 479 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 209/154 | 591 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 208/89 | 388 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 256/291 | 855 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 254/201 | 635 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 253/182 | 632 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 198/193 | 661 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 196/111 | 533 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 195/99 | 452 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 238/320 | 931 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 236/313 | 911 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 235/297 | 815 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 38 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 223/146 | 593 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 429 | 3000 ms | 0/0 | 42 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 3000 ms | 0/0 | 42 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 54 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | PROVIDER | 429 | 3000 ms | 0/0 | 52 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 52 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 46 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 44 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 37 ms |
