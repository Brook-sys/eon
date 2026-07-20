# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `nvidia/nemotron-nano-9b-v2`
- Runs: 11
- Compiled: 11
- Syntax valid: 0
- Semantically correct: 0
- Input tokens: 0
- Output tokens: 0
- Omitted facts: 0
- Errors (compile/provider/validation): 0/11/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `FAIL`
- Headline: Cognitive baseline FAIL for model "nvidia/nemotron-nano-9b-v2" on "cognitive-v1": correct=0/11 syntax_valid=0 (provider_errors=11 validation_errors=0).

### Notes

- `interpret:compiled=11`
- `interpret:errors_compile=0`
- `interpret:errors_provider=11`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=nvidia/nemotron-nano-9b-v2`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=0`
- `interpret:strongest_format=CHOICE rate=0/3`
- `interpret:syntax_valid=0`
- `interpret:total=11`
- `interpret:verdict=FAIL`
- `interpret:weakest_context=2048 rate=0/11`
- `interpret:weakest_format=CHOICE rate=0/3`
- `interpret:weakest_operation=CONFLICT rate=0/3`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 3 | 3 | 0 | 0 | 0 | 3 |
| EXTRACT | 3 | 3 | 0 | 0 | 0 | 3 |
| REPAIR | 2 | 2 | 0 | 0 | 0 | 2 |
| SYNTHESIZE | 3 | 3 | 0 | 0 | 0 | 3 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 3 | 3 | 0 | 0 | 0 | 3 |
| DELIMITED | 4 | 4 | 0 | 0 | 0 | 4 |
| JSON | 4 | 4 | 0 | 0 | 0 | 4 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 0 | 0 | 0 | 11 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 396 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
| extract-date | EXTRACT | JSON | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 128 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 404 | 0 ms | 0/0 | 120 ms |
