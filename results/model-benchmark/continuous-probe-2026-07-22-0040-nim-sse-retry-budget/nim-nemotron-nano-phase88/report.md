# Cognitive benchmark

- Fixture: `phase88-sse-retry-budget-v1`
- Model: `nvidia/nemotron-3-nano-30b-a3b`
- Runs: 1
- Compiled: 1
- Syntax valid: 1
- Semantically correct: 0
- Input tokens: 243
- Output tokens: 128
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `FAIL`
- Headline: Cognitive baseline FAIL for model "nvidia/nemotron-3-nano-30b-a3b" on "phase88-sse-retry-budget-v1": correct=0/1 syntax_valid=1 (provider_errors=0 validation_errors=0).

### Notes

- `interpret:compiled=1`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=0`
- `interpret:fixture=phase88-sse-retry-budget-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=nvidia/nemotron-3-nano-30b-a3b`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=0`
- `interpret:strongest_format=CHOICE rate=0/1`
- `interpret:syntax_valid=1`
- `interpret:total=1`
- `interpret:verdict=FAIL`
- `interpret:weakest_context=2048 rate=0/1`
- `interpret:weakest_format=CHOICE rate=0/1`
- `interpret:weakest_operation=SYNTHESIZE rate=0/1`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| SYNTHESIZE | 1 | 1 | 1 | 0 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 1 | 1 | 1 | 0 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 1 | 1 | 1 | 0 | 0 | 0 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| sse-retry-budget | SYNTHESIZE | CHOICE | 2048 | INCORRECT | 0 | 0 ms | 243/128 | 1521 ms |
