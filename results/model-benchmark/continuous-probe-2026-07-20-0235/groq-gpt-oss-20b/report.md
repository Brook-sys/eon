# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `openai/gpt-oss-20b`
- Runs: 11
- Compiled: 11
- Syntax valid: 10
- Semantically correct: 10
- Input tokens: 2296
- Output tokens: 1328
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/1
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "openai/gpt-oss-20b" on "cognitive-v1": correct=10/11 syntax_valid=10 (provider_errors=0 validation_errors=1).

### Notes

- `interpret:compiled=11`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=1`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=openai/gpt-oss-20b`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=10`
- `interpret:strongest_format=CHOICE rate=3/3`
- `interpret:syntax_valid=10`
- `interpret:total=11`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=10/11`
- `interpret:weakest_format=DELIMITED rate=3/4`
- `interpret:weakest_operation=REPAIR rate=1/2`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 3 | 3 | 3 | 3 | 0 | 0 |
| EXTRACT | 3 | 3 | 3 | 3 | 0 | 0 |
| REPAIR | 2 | 2 | 1 | 1 | 0 | 1 |
| SYNTHESIZE | 3 | 3 | 3 | 3 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 3 | 3 | 3 | 3 | 0 | 0 |
| DELIMITED | 4 | 4 | 3 | 3 | 0 | 1 |
| JSON | 4 | 4 | 4 | 4 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 10 | 10 | 0 | 1 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 211/139 | 401 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 209/137 | 456 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 208/93 | 465 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 198/77 | 438 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 196/84 | 440 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 195/86 | 338 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 225/235 | 697 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 223/83 | 360 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 222/119 | 394 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | VALIDATION | 0 | 0 ms | 205/202 | 559 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 204/73 | 360 ms |
