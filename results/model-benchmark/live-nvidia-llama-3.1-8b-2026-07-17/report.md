# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `meta/llama-3.1-8b-instruct`
- Runs: 33
- Compiled: 33
- Syntax valid: 8
- Semantically correct: 8
- Input tokens: 5670
- Output tokens: 4252
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/25

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "meta/llama-3.1-8b-instruct" on "cognitive-v1": correct=8/33 syntax_valid=8 (provider_errors=0 validation_errors=25).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=25`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=meta/llama-3.1-8b-instruct`
- `interpret:prefer_weaker_formats_or_smaller_ops_first`
- `interpret:semantically_correct=8`
- `interpret:syntax_valid=8`
- `interpret:total=33`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=4096 rate=2/11`
- `interpret:weakest_format=JSON rate=0/12`
- `interpret:weakest_operation=EXTRACT rate=0/9`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 2 | 2 | 0 | 7 |
| EXTRACT | 9 | 9 | 0 | 0 | 0 | 9 |
| REPAIR | 6 | 6 | 0 | 0 | 0 | 6 |
| SYNTHESIZE | 9 | 9 | 6 | 6 | 0 | 3 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 3 | 3 | 0 | 6 |
| DELIMITED | 12 | 12 | 5 | 5 | 0 | 7 |
| JSON | 12 | 12 | 0 | 0 | 0 | 12 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 3 | 3 | 0 | 8 |
| 4096 | 11 | 11 | 2 | 2 | 0 | 9 |
| 8192 | 11 | 11 | 3 | 3 | 0 | 8 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | VALIDATION | 175/180 | 1860 ms |
| extract-date | EXTRACT | CHOICE | 4096 | VALIDATION | 175/176 | 1362 ms |
| extract-date | EXTRACT | CHOICE | 8192 | VALIDATION | 175/193 | 2143 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | VALIDATION | 173/14 | 460 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | VALIDATION | 173/14 | 650 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | VALIDATION | 173/14 | 400 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 171/220 | 1601 ms |
| extract-date | EXTRACT | JSON | 4096 | VALIDATION | 171/217 | 1563 ms |
| extract-date | EXTRACT | JSON | 8192 | VALIDATION | 171/215 | 2329 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 161/9 | 366 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 161/9 | 378 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 161/9 | 471 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 159/10 | 392 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 159/10 | 375 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 159/10 | 361 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 157/256 | 1791 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | VALIDATION | 157/256 | 2264 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | VALIDATION | 157/256 | 1771 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 188/234 | 1684 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | VALIDATION | 188/256 | 2268 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | VALIDATION | 188/256 | 1802 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 186/14 | 513 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | VALIDATION | 186/13 | 374 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | CORRECT | 186/14 | 474 ms |
| detect-conflict | CONFLICT | JSON | 2048 | VALIDATION | 184/256 | 1687 ms |
| detect-conflict | CONFLICT | JSON | 4096 | VALIDATION | 184/256 | 1695 ms |
| detect-conflict | CONFLICT | JSON | 8192 | VALIDATION | 184/256 | 1832 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | VALIDATION | 169/17 | 398 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | VALIDATION | 169/17 | 393 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | VALIDATION | 169/17 | 397 ms |
| repair-anchor | REPAIR | JSON | 2048 | VALIDATION | 167/172 | 1301 ms |
| repair-anchor | REPAIR | JSON | 4096 | VALIDATION | 167/203 | 1429 ms |
| repair-anchor | REPAIR | JSON | 8192 | VALIDATION | 167/203 | 1487 ms |
