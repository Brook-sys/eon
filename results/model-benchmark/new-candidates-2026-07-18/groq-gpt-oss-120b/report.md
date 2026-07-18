# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `openai/gpt-oss-120b`
- Runs: 11
- Compiled: 11
- Syntax valid: 10
- Semantically correct: 10
- Input tokens: 2071
- Output tokens: 1313
- Omitted facts: 0
- Errors (compile/provider/validation): 0/1/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "openai/gpt-oss-120b" on "cognitive-v1": correct=10/11 syntax_valid=10 (provider_errors=1 validation_errors=0).

### Notes

- `interpret:compiled=11`
- `interpret:errors_compile=0`
- `interpret:errors_provider=1`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=openai/gpt-oss-120b`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=10`
- `interpret:strongest_format=DELIMITED rate=4/4`
- `interpret:syntax_valid=10`
- `interpret:total=11`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=10/11`
- `interpret:weakest_format=CHOICE rate=2/3`
- `interpret:weakest_operation=CONFLICT rate=2/3`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 3 | 3 | 2 | 2 | 0 | 1 |
| EXTRACT | 3 | 3 | 3 | 3 | 0 | 0 |
| REPAIR | 2 | 2 | 2 | 2 | 0 | 0 |
| SYNTHESIZE | 3 | 3 | 3 | 3 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 3 | 3 | 2 | 2 | 0 | 1 |
| DELIMITED | 4 | 4 | 4 | 4 | 0 | 0 |
| JSON | 4 | 4 | 4 | 4 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 10 | 10 | 0 | 1 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 211/142 | 622 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 209/142 | 1064 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 208/103 | 496 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 198/208 | 683 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 196/118 | 542 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 195/99 | 435 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 0 | 0 ms | 0/0 | 771 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 223/146 | 545 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 222/96 | 441 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 205/171 | 596 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 204/88 | 430 ms |
