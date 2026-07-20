# Cognitive benchmark

- Fixture: `cognitive-tool-v1`
- Model: `meta/llama-3.1-8b-instruct`
- Runs: 1
- Compiled: 1
- Syntax valid: 1
- Semantically correct: 1
- Input tokens: 468
- Output tokens: 38
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PASS`
- Headline: Cognitive baseline PASS for model "meta/llama-3.1-8b-instruct" on "cognitive-tool-v1": correct=1/1 syntax_valid=1 (provider_errors=0 validation_errors=0).

### Notes

- `interpret:compiled=1`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-tool-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=meta/llama-3.1-8b-instruct`
- `interpret:semantically_correct=1`
- `interpret:strongest_format=JSON rate=1/1`
- `interpret:syntax_valid=1`
- `interpret:total=1`
- `interpret:verdict=PASS`
- `interpret:weakest_context=4096 rate=1/1`
- `interpret:weakest_format=JSON rate=1/1`
- `interpret:weakest_operation=SYNTHESIZE rate=1/1`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| SYNTHESIZE | 1 | 1 | 1 | 1 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| JSON | 1 | 1 | 1 | 1 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 4096 | 1 | 1 | 1 | 1 | 0 | 0 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| tool-search-single | SYNTHESIZE | JSON | 4096 | CORRECT | 0 | 0 ms | 468/38 | 1092 ms |
