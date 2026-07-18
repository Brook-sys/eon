# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `openai/gpt-oss-20b`
- Runs: 33
- Compiled: 33
- Syntax valid: 19
- Semantically correct: 19
- Input tokens: 3873
- Output tokens: 1967
- Omitted facts: 0
- Errors (compile/provider/validation): 0/14/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "openai/gpt-oss-20b" on "cognitive-v1": correct=19/33 syntax_valid=19 (provider_errors=14 validation_errors=0).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=14`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=openai/gpt-oss-20b`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=19`
- `interpret:strongest_format=CHOICE rate=6/9`
- `interpret:syntax_valid=19`
- `interpret:total=33`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=6/11`
- `interpret:weakest_format=DELIMITED rate=6/12`
- `interpret:weakest_operation=REPAIR rate=0/6`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 1 | 1 | 0 | 8 |
| EXTRACT | 9 | 9 | 9 | 9 | 0 | 0 |
| REPAIR | 6 | 6 | 0 | 0 | 0 | 6 |
| SYNTHESIZE | 9 | 9 | 9 | 9 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 6 | 6 | 0 | 3 |
| DELIMITED | 12 | 12 | 6 | 6 | 0 | 6 |
| JSON | 12 | 12 | 7 | 7 | 0 | 5 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 6 | 6 | 0 | 5 |
| 4096 | 11 | 11 | 7 | 7 | 0 | 4 |
| 8192 | 11 | 11 | 6 | 6 | 0 | 5 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 211/139 | 483 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 211/139 | 416 ms |
| extract-date | EXTRACT | CHOICE | 8192 | CORRECT | 211/139 | 343 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 209/137 | 381 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 209/137 | 406 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 209/137 | 377 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 208/93 | 303 ms |
| extract-date | EXTRACT | JSON | 4096 | CORRECT | 208/93 | 337 ms |
| extract-date | EXTRACT | JSON | 8192 | CORRECT | 208/93 | 450 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 198/77 | 357 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 198/77 | 279 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 198/77 | 278 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 196/84 | 319 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 196/84 | 416 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 196/84 | 440 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 195/86 | 302 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | CORRECT | 195/86 | 414 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | CORRECT | 195/86 | 413 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 0/0 | 31 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | PROVIDER | 0/0 | 35 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | PROVIDER | 0/0 | 35 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 0/0 | 35 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | PROVIDER | 0/0 | 33 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | PROVIDER | 0/0 | 35 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 0/0 | 32 ms |
| detect-conflict | CONFLICT | JSON | 4096 | CORRECT | 222/119 | 412 ms |
| detect-conflict | CONFLICT | JSON | 8192 | PROVIDER | 0/0 | 36 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 0/0 | 35 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | PROVIDER | 0/0 | 38 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | PROVIDER | 0/0 | 33 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 0/0 | 34 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 0/0 | 32 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 0/0 | 29 ms |
