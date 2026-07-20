# Cognitive benchmark

- Fixture: `cognitive-tool-v1`
- Model: `llama-3.3-70b-versatile`
- Runs: 1
- Compiled: 1
- Syntax valid: 0
- Semantically correct: 0
- Input tokens: 0
- Output tokens: 0
- Omitted facts: 0
- Errors (compile/provider/validation): 0/1/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `FAIL`
- Headline: Cognitive baseline FAIL for model "llama-3.3-70b-versatile" on "cognitive-tool-v1": correct=0/1 syntax_valid=0 (provider_errors=1 validation_errors=0).

### Notes

- `interpret:compiled=1`
- `interpret:errors_compile=0`
- `interpret:errors_provider=1`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-tool-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.3-70b-versatile`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=0`
- `interpret:strongest_format=JSON rate=0/1`
- `interpret:syntax_valid=0`
- `interpret:total=1`
- `interpret:verdict=FAIL`
- `interpret:weakest_context=4096 rate=0/1`
- `interpret:weakest_format=JSON rate=0/1`
- `interpret:weakest_operation=SYNTHESIZE rate=0/1`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| SYNTHESIZE | 1 | 1 | 0 | 0 | 0 | 1 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| JSON | 1 | 1 | 0 | 0 | 0 | 1 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 4096 | 1 | 1 | 0 | 0 | 0 | 1 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| tool-search-single | SYNTHESIZE | JSON | 4096 | PROVIDER | 401 | 0 ms | 0/0 | 216 ms |
