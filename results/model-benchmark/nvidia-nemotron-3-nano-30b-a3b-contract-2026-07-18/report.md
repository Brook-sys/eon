# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `nvidia/nemotron-3-nano-30b-a3b`
- Runs: 33
- Compiled: 33
- Syntax valid: 16
- Semantically correct: 16
- Input tokens: 5637
- Output tokens: 7495
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/17

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "nvidia/nemotron-3-nano-30b-a3b" on "cognitive-v1": correct=16/33 syntax_valid=16 (provider_errors=0 validation_errors=17).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=17`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=nvidia/nemotron-3-nano-30b-a3b`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=16`
- `interpret:strongest_format=JSON rate=10/12`
- `interpret:syntax_valid=16`
- `interpret:total=33`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=5/11`
- `interpret:weakest_format=CHOICE rate=2/9`
- `interpret:weakest_operation=SYNTHESIZE rate=1/9`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 3 | 3 | 0 | 6 |
| EXTRACT | 9 | 9 | 8 | 8 | 0 | 1 |
| REPAIR | 6 | 6 | 4 | 4 | 0 | 2 |
| SYNTHESIZE | 9 | 9 | 1 | 1 | 0 | 8 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 2 | 2 | 0 | 7 |
| DELIMITED | 12 | 12 | 4 | 4 | 0 | 8 |
| JSON | 12 | 12 | 10 | 10 | 0 | 2 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 5 | 5 | 0 | 6 |
| 4096 | 11 | 11 | 6 | 6 | 0 | 5 |
| 8192 | 11 | 11 | 5 | 5 | 0 | 6 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 180/233 | 5997 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 180/231 | 3812 ms |
| extract-date | EXTRACT | CHOICE | 8192 | VALIDATION | 180/256 | 2572 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 178/247 | 2406 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 178/234 | 2114 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 178/200 | 1981 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 177/232 | 2585 ms |
| extract-date | EXTRACT | JSON | 4096 | CORRECT | 177/229 | 2020 ms |
| extract-date | EXTRACT | JSON | 8192 | CORRECT | 177/232 | 2849 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | VALIDATION | 156/256 | 2383 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | VALIDATION | 156/256 | 3261 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | VALIDATION | 156/256 | 3549 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | VALIDATION | 154/256 | 5273 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | VALIDATION | 154/256 | 2568 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | VALIDATION | 154/256 | 2625 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 153/256 | 1958 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | VALIDATION | 153/256 | 2300 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | CORRECT | 153/256 | 3225 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 189/174 | 2296 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | VALIDATION | 189/226 | 2159 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | VALIDATION | 189/256 | 3215 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | VALIDATION | 187/153 | 1299 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | VALIDATION | 187/203 | 2780 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | VALIDATION | 187/256 | 2860 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 186/173 | 1698 ms |
| detect-conflict | CONFLICT | JSON | 4096 | CORRECT | 186/256 | 1999 ms |
| detect-conflict | CONFLICT | JSON | 8192 | CORRECT | 186/172 | 1407 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | VALIDATION | 160/256 | 3888 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | CORRECT | 160/240 | 2354 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | VALIDATION | 160/256 | 3063 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 159/147 | 1346 ms |
| repair-anchor | REPAIR | JSON | 4096 | CORRECT | 159/166 | 2231 ms |
| repair-anchor | REPAIR | JSON | 8192 | CORRECT | 159/163 | 1464 ms |
