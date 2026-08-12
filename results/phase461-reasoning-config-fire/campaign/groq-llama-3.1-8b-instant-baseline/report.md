# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `llama-3.1-8b-instant`
- Runs: 22
- Compiled: 22
- Syntax valid: 5
- Semantically correct: 5
- Input tokens: 1336
- Output tokens: 702
- Omitted facts: 0
- Errors (compile/provider/validation): 0/15/2
- Rate limited / timed out: 15/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.1-8b-instant" on "cognitive-v2": correct=5/22 syntax_valid=5 (provider_errors=15 validation_errors=2).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=15`
- `interpret:errors_validation=2`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.1-8b-instant`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=5`
- `interpret:strongest_format=CHOICE rate=3/6`
- `interpret:syntax_valid=5`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=5/22`
- `interpret:weakest_format=JSON rate=0/8`
- `interpret:weakest_operation=CONFLICT rate=0/6`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 0 | 0 | 0 | 6 |
| EXTRACT | 6 | 6 | 4 | 4 | 0 | 2 |
| REPAIR | 4 | 4 | 0 | 0 | 0 | 4 |
| SYNTHESIZE | 6 | 6 | 1 | 1 | 0 | 5 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 3 | 3 | 0 | 3 |
| DELIMITED | 8 | 8 | 2 | 2 | 0 | 6 |
| JSON | 8 | 8 | 0 | 0 | 0 | 8 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 5 | 5 | 0 | 17 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 175/46 | 334 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 173/14 | 217 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 172/301 | 627 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 220/26 | 243 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 218/14 | 216 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 217/292 | 502 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 161/9 | 298 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 32 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 31 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 32 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 32 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 31 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 31 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 32 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 29 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 32 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 31 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 32 ms |
