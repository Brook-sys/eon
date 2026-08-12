# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `openai/gpt-oss-20b`
- Runs: 22
- Compiled: 22
- Syntax valid: 20
- Semantically correct: 20
- Input tokens: 4695
- Output tokens: 2448
- Omitted facts: 0
- Errors (compile/provider/validation): 0/1/1
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "openai/gpt-oss-20b" on "cognitive-v2": correct=20/22 syntax_valid=20 (provider_errors=1 validation_errors=1).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=1`
- `interpret:errors_validation=1`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=openai/gpt-oss-20b`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=20`
- `interpret:strongest_format=CHOICE rate=6/6`
- `interpret:syntax_valid=20`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=20/22`
- `interpret:weakest_format=DELIMITED rate=7/8`
- `interpret:weakest_operation=REPAIR rate=2/4`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 6 | 6 | 0 | 0 |
| EXTRACT | 6 | 6 | 6 | 6 | 0 | 0 |
| REPAIR | 4 | 4 | 2 | 2 | 0 | 2 |
| SYNTHESIZE | 6 | 6 | 6 | 6 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 6 | 6 | 0 | 0 |
| DELIMITED | 8 | 8 | 7 | 7 | 0 | 1 |
| JSON | 8 | 8 | 7 | 7 | 0 | 1 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 20 | 20 | 0 | 2 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 211/138 | 518 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 209/137 | 356 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 208/93 | 474 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 256/111 | 417 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 254/90 | 344 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 253/174 | 555 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 198/77 | 363 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 196/84 | 466 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 195/86 | 322 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 238/91 | 438 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 236/86 | 333 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 235/113 | 411 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 225/150 | 406 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 223/77 | 347 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 222/122 | 372 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 235/128 | 343 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 233/126 | 382 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 232/127 | 359 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 205/79 | 405 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 204/73 | 408 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | VALIDATION | 0 | 0 ms | 227/286 | 723 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 0 | 0 ms | 0/0 | 918 ms |
