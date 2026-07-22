# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `offline-compile`
- Runs: 66
- Compiled: 66
- Syntax valid: 0
- Semantically correct: 0
- Input tokens: 13323
- Output tokens: 0
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `offline-compile`
- Verdict: `UNSCORED`
- Headline: Offline compile-only matrix on "cognitive-v2": compiled=66/66 compile_errors=0 (no provider answers scored).

### Notes

- `interpret:compiled=66`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=offline-compile`
- `interpret:model=offline-compile`
- `interpret:no_provider_answers_scored`
- `interpret:prompt_budget_matrix_compiles`
- `interpret:semantically_correct=0`
- `interpret:strongest_format=CHOICE rate=0/18`
- `interpret:syntax_valid=0`
- `interpret:total=66`
- `interpret:verdict=UNSCORED`
- `interpret:weakest_context=2048 rate=0/22`
- `interpret:weakest_format=CHOICE rate=0/18`
- `interpret:weakest_operation=CONFLICT rate=0/18`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 18 | 18 | 0 | 0 | 0 | 0 |
| EXTRACT | 18 | 18 | 0 | 0 | 0 | 0 |
| REPAIR | 12 | 12 | 0 | 0 | 0 | 0 |
| SYNTHESIZE | 18 | 18 | 0 | 0 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 18 | 18 | 0 | 0 | 0 | 0 |
| DELIMITED | 24 | 24 | 0 | 0 | 0 | 0 |
| JSON | 24 | 24 | 0 | 0 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 0 | 0 | 0 | 0 |
| 4096 | 22 | 22 | 0 | 0 | 0 | 0 |
| 8192 | 22 | 22 | 0 | 0 | 0 | 0 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-date | EXTRACT | CHOICE | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-date | EXTRACT | CHOICE | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-date | EXTRACT | JSON | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-date | EXTRACT | JSON | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-date | EXTRACT | JSON | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | CHOICE | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | CHOICE | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | JSON | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| extract-qualified-date | EXTRACT | JSON | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | JSON | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | JSON | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| detect-conflict | CONFLICT | JSON | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | JSON | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| distinguish-temporal-change | CONFLICT | JSON | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-anchor | REPAIR | JSON | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-anchor | REPAIR | JSON | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-anchor | REPAIR | JSON | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-unrecoverable | REPAIR | JSON | 4096 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
| repair-unrecoverable | REPAIR | JSON | 8192 | INCORRECT | 0 | 0 ms | 0/0 | 0 ms |
