# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: ``
- Runs: 33
- Compiled: 33
- Syntax valid: 0
- Semantically correct: 0
- Input tokens: 0
- Output tokens: 0
- Omitted facts: 0
- Errors (compile/provider/validation): 0/33/0

## Interpretation

- Kind: `offline-compile`
- Verdict: `FAIL`
- Headline: Offline compile-only matrix on "cognitive-v1": compiled=33/33 compile_errors=0 (no provider answers scored).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=33`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=offline-compile`
- `interpret:model=`
- `interpret:no_provider_answers_scored`
- `interpret:prompt_budget_matrix_compiles`
- `interpret:semantically_correct=0`
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
| extract-date | EXTRACT | CHOICE | 2048 | PROVIDER | 0/0 | 90059 ms |
| extract-date | EXTRACT | CHOICE | 4096 | PROVIDER | 0/0 | 90000 ms |
| extract-date | EXTRACT | CHOICE | 8192 | PROVIDER | 0/0 | 90004 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | PROVIDER | 0/0 | 90029 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | PROVIDER | 0/0 | 90000 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | PROVIDER | 0/0 | 29916 ms |
| extract-date | EXTRACT | JSON | 2048 | PROVIDER | 0/0 | 0 ms |
| extract-date | EXTRACT | JSON | 4096 | PROVIDER | 0/0 | 0 ms |
| extract-date | EXTRACT | JSON | 8192 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | PROVIDER | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | JSON | 4096 | PROVIDER | 0/0 | 0 ms |
| detect-conflict | CONFLICT | JSON | 8192 | PROVIDER | 0/0 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 0/0 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | PROVIDER | 0/0 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | PROVIDER | 0/0 | 0 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 0/0 | 0 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 0/0 | 0 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 0/0 | 0 ms |
