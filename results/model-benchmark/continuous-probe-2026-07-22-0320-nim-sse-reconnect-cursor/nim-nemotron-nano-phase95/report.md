# Cognitive benchmark

- Fixture: `phase95-sse-reconnect-cursor`
- Model: `nvidia/nemotron-3-nano-30b-a3b`
- Runs: 1
- Compiled: 1
- Syntax valid: 0
- Semantically correct: 0
- Input tokens: 223
- Output tokens: 128
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/1
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `FAIL`
- Headline: Cognitive baseline FAIL for model "nvidia/nemotron-3-nano-30b-a3b" on "phase95-sse-reconnect-cursor": correct=0/1 syntax_valid=0 (provider_errors=0 validation_errors=1).

### Notes

- `interpret:compiled=1`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=1`
- `interpret:fixture=phase95-sse-reconnect-cursor`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=nvidia/nemotron-3-nano-30b-a3b`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=0`
- `interpret:strongest_format=CHOICE rate=0/1`
- `interpret:syntax_valid=0`
- `interpret:total=1`
- `interpret:verdict=FAIL`
- `interpret:weakest_context=2048 rate=0/1`
- `interpret:weakest_format=CHOICE rate=0/1`
- `interpret:weakest_operation=SYNTHESIZE rate=0/1`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| SYNTHESIZE | 1 | 1 | 0 | 0 | 0 | 1 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 1 | 1 | 0 | 0 | 0 | 1 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 1 | 1 | 0 | 0 | 0 | 1 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| sse-reconnect-cursor-integrity | SYNTHESIZE | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 223/128 | 1844 ms |
