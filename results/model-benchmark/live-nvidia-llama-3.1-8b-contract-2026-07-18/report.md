# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `meta/llama-3.1-8b-instruct`
- Runs: 33
- Compiled: 33
- Syntax valid: 15
- Semantically correct: 15
- Input tokens: 5670
- Output tokens: 4282
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/18

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "meta/llama-3.1-8b-instruct" on "cognitive-v1": correct=15/33 syntax_valid=15 (provider_errors=0 validation_errors=18).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=18`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=meta/llama-3.1-8b-instruct`
- `interpret:prefer_weaker_formats_or_smaller_ops_first`
- `interpret:semantically_correct=15`
- `interpret:syntax_valid=15`
- `interpret:total=33`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=4096 rate=4/11`
- `interpret:weakest_format=JSON rate=1/12`
- `interpret:weakest_operation=CONFLICT rate=2/9`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 2 | 2 | 0 | 7 |
| EXTRACT | 9 | 9 | 3 | 3 | 0 | 6 |
| REPAIR | 6 | 6 | 4 | 4 | 0 | 2 |
| SYNTHESIZE | 9 | 9 | 6 | 6 | 0 | 3 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 3 | 3 | 0 | 6 |
| DELIMITED | 12 | 12 | 11 | 11 | 0 | 1 |
| JSON | 12 | 12 | 1 | 1 | 0 | 11 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 5 | 5 | 0 | 6 |
| 4096 | 11 | 11 | 4 | 4 | 0 | 7 |
| 8192 | 11 | 11 | 6 | 6 | 0 | 5 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | VALIDATION | 175/178 | 2166 ms |
| extract-date | EXTRACT | CHOICE | 4096 | VALIDATION | 175/178 | 1298 ms |
| extract-date | EXTRACT | CHOICE | 8192 | VALIDATION | 175/175 | 1685 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 173/14 | 428 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 173/14 | 436 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 173/14 | 389 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 171/256 | 1833 ms |
| extract-date | EXTRACT | JSON | 4096 | VALIDATION | 171/256 | 1618 ms |
| extract-date | EXTRACT | JSON | 8192 | VALIDATION | 171/225 | 4068 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 161/9 | 344 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 161/9 | 344 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 161/9 | 381 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 159/9 | 389 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 159/9 | 388 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 159/9 | 388 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 157/256 | 1803 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | VALIDATION | 157/256 | 2339 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | VALIDATION | 157/256 | 2299 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 188/256 | 1716 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | VALIDATION | 188/256 | 2732 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | VALIDATION | 188/256 | 2188 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 186/13 | 387 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | VALIDATION | 186/34 | 490 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | CORRECT | 186/13 | 398 ms |
| detect-conflict | CONFLICT | JSON | 2048 | VALIDATION | 184/256 | 1741 ms |
| detect-conflict | CONFLICT | JSON | 4096 | VALIDATION | 184/256 | 1730 ms |
| detect-conflict | CONFLICT | JSON | 8192 | VALIDATION | 184/256 | 1716 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 169/17 | 399 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | CORRECT | 169/17 | 401 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | CORRECT | 169/17 | 406 ms |
| repair-anchor | REPAIR | JSON | 2048 | VALIDATION | 167/183 | 1518 ms |
| repair-anchor | REPAIR | JSON | 4096 | VALIDATION | 167/256 | 1748 ms |
| repair-anchor | REPAIR | JSON | 8192 | CORRECT | 167/64 | 641 ms |
