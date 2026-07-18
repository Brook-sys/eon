# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `groq/compound-mini`
- Runs: 11
- Compiled: 11
- Syntax valid: 4
- Semantically correct: 4
- Input tokens: 2865
- Output tokens: 846
- Omitted facts: 0
- Errors (compile/provider/validation): 0/7/0
- Rate limited / timed out: 7/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "groq/compound-mini" on "cognitive-v1": correct=4/11 syntax_valid=4 (provider_errors=7 validation_errors=0).

### Notes

- `interpret:compiled=11`
- `interpret:errors_compile=0`
- `interpret:errors_provider=7`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=groq/compound-mini`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=4`
- `interpret:strongest_format=DELIMITED rate=2/4`
- `interpret:syntax_valid=4`
- `interpret:total=11`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=4/11`
- `interpret:weakest_format=JSON rate=1/4`
- `interpret:weakest_operation=SYNTHESIZE rate=0/3`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 3 | 3 | 1 | 1 | 0 | 2 |
| EXTRACT | 3 | 3 | 2 | 2 | 0 | 1 |
| REPAIR | 2 | 2 | 1 | 1 | 0 | 1 |
| SYNTHESIZE | 3 | 3 | 0 | 0 | 0 | 3 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 3 | 3 | 1 | 1 | 0 | 2 |
| DELIMITED | 4 | 4 | 2 | 2 | 0 | 2 |
| JSON | 4 | 4 | 1 | 1 | 0 | 3 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 4 | 4 | 0 | 7 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 715/231 | 1094 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 711/187 | 962 ms |
| extract-date | EXTRACT | JSON | 2048 | PROVIDER | 429 | 0 ms | 0/0 | 740 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 429 | 0 ms | 0/0 | 983 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 429 | 0 ms | 0/0 | 330 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | PROVIDER | 429 | 0 ms | 0/0 | 39 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 0 ms | 0/0 | 824 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 0 ms | 0/0 | 39 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 0 | 0 ms | 736/278 | 1258 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 703/150 | 947 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 429 | 0 ms | 0/0 | 31 ms |
