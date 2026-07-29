# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `meta/llama-3.1-8b-instruct`
- Runs: 22
- Compiled: 22
- Syntax valid: 14
- Semantically correct: 12
- Input tokens: 4114
- Output tokens: 1995
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/8
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "meta/llama-3.1-8b-instruct" on "cognitive-v2": correct=12/22 syntax_valid=14 (provider_errors=0 validation_errors=8).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=8`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=meta/llama-3.1-8b-instruct`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=12`
- `interpret:strongest_format=DELIMITED rate=7/8`
- `interpret:syntax_valid=14`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=12/22`
- `interpret:weakest_format=JSON rate=2/8`
- `interpret:weakest_operation=CONFLICT rate=3/6`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 3 | 3 | 0 | 3 |
| EXTRACT | 6 | 6 | 3 | 3 | 0 | 3 |
| REPAIR | 4 | 4 | 3 | 3 | 0 | 1 |
| SYNTHESIZE | 6 | 6 | 5 | 3 | 0 | 1 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 4 | 3 | 0 | 2 |
| DELIMITED | 8 | 8 | 8 | 7 | 0 | 0 |
| JSON | 8 | 8 | 2 | 2 | 0 | 6 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 14 | 12 | 0 | 8 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 175/14 | 734 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 173/14 | 429 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 171/256 | 6608 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 220/229 | 4292 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 218/14 | 432 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 216/242 | 2523 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 161/9 | 389 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 159/9 | 393 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 0 | 0 ms | 157/256 | 2667 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | INCORRECT | 0 | 0 ms | 201/8 | 370 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 199/8 | 382 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 197/52 | 723 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 188/17 | 464 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 186/13 | 466 ms |
| detect-conflict | CONFLICT | JSON | 2048 | VALIDATION | 0 | 0 ms | 184/256 | 3175 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 199/14 | 768 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 197/14 | 441 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | VALIDATION | 0 | 0 ms | 195/256 | 2847 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 169/17 | 475 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 167/30 | 511 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 192/11 | 412 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | VALIDATION | 0 | 0 ms | 190/256 | 3836 ms |
