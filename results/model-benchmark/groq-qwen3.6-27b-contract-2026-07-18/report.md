# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `qwen/qwen3.6-27b`
- Runs: 33
- Compiled: 33
- Syntax valid: 0
- Semantically correct: 0
- Input tokens: 3465
- Output tokens: 5632
- Omitted facts: 0
- Errors (compile/provider/validation): 0/11/22

## Interpretation

- Kind: `live`
- Verdict: `FAIL`
- Headline: Cognitive baseline FAIL for model "qwen/qwen3.6-27b" on "cognitive-v1": correct=0/33 syntax_valid=0 (provider_errors=11 validation_errors=22).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=11`
- `interpret:errors_validation=22`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=qwen/qwen3.6-27b`
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
| extract-date | EXTRACT | CHOICE | 2048 | VALIDATION | 170/256 | 789 ms |
| extract-date | EXTRACT | CHOICE | 4096 | VALIDATION | 170/256 | 795 ms |
| extract-date | EXTRACT | CHOICE | 8192 | VALIDATION | 170/256 | 3740 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | VALIDATION | 168/256 | 699 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | VALIDATION | 168/256 | 787 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | VALIDATION | 168/256 | 776 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 166/256 | 687 ms |
| extract-date | EXTRACT | JSON | 4096 | VALIDATION | 166/256 | 721 ms |
| extract-date | EXTRACT | JSON | 8192 | VALIDATION | 166/256 | 795 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | VALIDATION | 143/256 | 680 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | VALIDATION | 143/256 | 972 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | VALIDATION | 143/256 | 1395 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | VALIDATION | 141/256 | 688 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | VALIDATION | 141/256 | 751 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | VALIDATION | 141/256 | 833 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 139/256 | 703 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | VALIDATION | 139/256 | 681 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | VALIDATION | 139/256 | 838 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 178/256 | 687 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | VALIDATION | 178/256 | 718 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | VALIDATION | 178/256 | 987 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 0/0 | 33 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | PROVIDER | 0/0 | 35 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | PROVIDER | 0/0 | 34 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 0/0 | 32 ms |
| detect-conflict | CONFLICT | JSON | 4096 | PROVIDER | 0/0 | 30 ms |
| detect-conflict | CONFLICT | JSON | 8192 | PROVIDER | 0/0 | 35 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | VALIDATION | 150/256 | 710 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | PROVIDER | 0/0 | 35 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | PROVIDER | 0/0 | 33 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 0/0 | 34 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 0/0 | 34 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 0/0 | 30 ms |
