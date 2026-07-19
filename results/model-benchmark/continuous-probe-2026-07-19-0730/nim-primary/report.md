# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `mistralai/mistral-small-4-119b-2603`
- Runs: 22
- Compiled: 22
- Syntax valid: 0
- Semantically correct: 0
- Input tokens: 0
- Output tokens: 0
- Omitted facts: 0
- Errors (compile/provider/validation): 0/22/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `FAIL`
- Headline: Cognitive baseline FAIL for model "mistralai/mistral-small-4-119b-2603" on "cognitive-v2": correct=0/22 syntax_valid=0 (provider_errors=22 validation_errors=0).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=22`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=mistralai/mistral-small-4-119b-2603`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=0`
- `interpret:strongest_format=CHOICE rate=0/6`
- `interpret:syntax_valid=0`
- `interpret:total=22`
- `interpret:verdict=FAIL`
- `interpret:weakest_context=2048 rate=0/22`
- `interpret:weakest_format=CHOICE rate=0/6`
- `interpret:weakest_operation=CONFLICT rate=0/6`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 0 | 0 | 0 | 6 |
| EXTRACT | 6 | 6 | 0 | 0 | 0 | 6 |
| REPAIR | 4 | 4 | 0 | 0 | 0 | 4 |
| SYNTHESIZE | 6 | 6 | 0 | 0 | 0 | 6 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 0 | 0 | 0 | 6 |
| DELIMITED | 8 | 8 | 0 | 0 | 0 | 8 |
| JSON | 8 | 8 | 0 | 0 | 0 | 8 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 0 | 0 | 0 | 22 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 399 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 124 ms |
| extract-date | EXTRACT | JSON | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 124 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 124 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 124 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 129 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 122 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 224 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 125 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 245 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 123 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 401 | 0 ms | 0/0 | 122 ms |
