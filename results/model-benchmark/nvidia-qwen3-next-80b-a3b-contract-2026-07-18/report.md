# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `qwen/qwen3-next-80b-a3b-instruct`
- Runs: 33
- Compiled: 33
- Syntax valid: 0
- Semantically correct: 0
- Input tokens: 0
- Output tokens: 0
- Omitted facts: 0
- Errors (compile/provider/validation): 0/33/0

## Interpretation

- Kind: `live`
- Verdict: `FAIL`
- Headline: Cognitive baseline FAIL for model "qwen/qwen3-next-80b-a3b-instruct" on "cognitive-v1": correct=0/33 syntax_valid=0 (provider_errors=33 validation_errors=0).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=33`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=qwen/qwen3-next-80b-a3b-instruct`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=0`
- `interpret:strongest_format=CHOICE rate=0/9`
- `interpret:syntax_valid=0`
- `interpret:total=33`
- `interpret:verdict=FAIL`
- `interpret:weakest_context=2048 rate=0/11`
- `interpret:weakest_format=CHOICE rate=0/9`
- `interpret:weakest_operation=CONFLICT rate=0/9`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 0 | 0 | 0 | 9 |
| EXTRACT | 9 | 9 | 0 | 0 | 0 | 9 |
| REPAIR | 6 | 6 | 0 | 0 | 0 | 6 |
| SYNTHESIZE | 9 | 9 | 0 | 0 | 0 | 9 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 0 | 0 | 0 | 9 |
| DELIMITED | 12 | 12 | 0 | 0 | 0 | 12 |
| JSON | 12 | 12 | 0 | 0 | 0 | 12 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 0 | 0 | 0 | 11 |
| 4096 | 11 | 11 | 0 | 0 | 0 | 11 |
| 8192 | 11 | 11 | 0 | 0 | 0 | 11 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | PROVIDER | 0/0 | 396 ms |
| extract-date | EXTRACT | CHOICE | 4096 | PROVIDER | 0/0 | 127 ms |
| extract-date | EXTRACT | CHOICE | 8192 | PROVIDER | 0/0 | 125 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | PROVIDER | 0/0 | 125 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | PROVIDER | 0/0 | 124 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | PROVIDER | 0/0 | 125 ms |
| extract-date | EXTRACT | JSON | 2048 | PROVIDER | 0/0 | 125 ms |
| extract-date | EXTRACT | JSON | 4096 | PROVIDER | 0/0 | 124 ms |
| extract-date | EXTRACT | JSON | 8192 | PROVIDER | 0/0 | 132 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 0/0 | 138 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | PROVIDER | 0/0 | 133 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | PROVIDER | 0/0 | 131 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 0/0 | 130 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | PROVIDER | 0/0 | 135 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | PROVIDER | 0/0 | 135 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | PROVIDER | 0/0 | 125 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | PROVIDER | 0/0 | 125 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | PROVIDER | 0/0 | 125 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 0/0 | 131 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | PROVIDER | 0/0 | 126 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | PROVIDER | 0/0 | 125 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 0/0 | 124 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | PROVIDER | 0/0 | 124 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | PROVIDER | 0/0 | 90000 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 0/0 | 139 ms |
| detect-conflict | CONFLICT | JSON | 4096 | PROVIDER | 0/0 | 126 ms |
| detect-conflict | CONFLICT | JSON | 8192 | PROVIDER | 0/0 | 137 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 0/0 | 136 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | PROVIDER | 0/0 | 127 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | PROVIDER | 0/0 | 136 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 0/0 | 124 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 0/0 | 130 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 0/0 | 137 ms |
