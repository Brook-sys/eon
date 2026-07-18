# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `meta/llama-3.1-8b-instruct`
- Runs: 33
- Compiled: 33
- Syntax valid: 7
- Semantically correct: 7
- Input tokens: 5670
- Output tokens: 4363
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/26

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "meta/llama-3.1-8b-instruct" on "cognitive-v1": correct=7/33 syntax_valid=7 (provider_errors=0 validation_errors=26).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=26`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=meta/llama-3.1-8b-instruct`
- `interpret:prefer_weaker_formats_or_smaller_ops_first`
- `interpret:semantically_correct=7`
- `interpret:syntax_valid=7`
- `interpret:total=33`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=4096 rate=2/11`
- `interpret:weakest_format=JSON rate=0/12`
- `interpret:weakest_operation=EXTRACT rate=0/9`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 1 | 1 | 0 | 8 |
| EXTRACT | 9 | 9 | 0 | 0 | 0 | 9 |
| REPAIR | 6 | 6 | 0 | 0 | 0 | 6 |
| SYNTHESIZE | 9 | 9 | 6 | 6 | 0 | 3 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 3 | 3 | 0 | 6 |
| DELIMITED | 12 | 12 | 4 | 4 | 0 | 8 |
| JSON | 12 | 12 | 0 | 0 | 0 | 12 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 3 | 3 | 0 | 8 |
| 4096 | 11 | 11 | 2 | 2 | 0 | 9 |
| 8192 | 11 | 11 | 2 | 2 | 0 | 9 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | VALIDATION | 175/199 | 1680 ms |
| extract-date | EXTRACT | CHOICE | 4096 | VALIDATION | 175/199 | 1636 ms |
| extract-date | EXTRACT | CHOICE | 8192 | VALIDATION | 175/199 | 1389 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | VALIDATION | 173/14 | 376 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | VALIDATION | 173/14 | 381 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | VALIDATION | 173/14 | 398 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 171/256 | 1679 ms |
| extract-date | EXTRACT | JSON | 4096 | VALIDATION | 171/256 | 1568 ms |
| extract-date | EXTRACT | JSON | 8192 | VALIDATION | 171/256 | 1692 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 161/9 | 366 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 161/9 | 350 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 161/9 | 347 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 159/10 | 359 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 159/10 | 421 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 159/10 | 363 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 157/256 | 1509 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | VALIDATION | 157/256 | 1749 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | VALIDATION | 157/256 | 1618 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 188/256 | 1609 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | VALIDATION | 188/256 | 2427 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | VALIDATION | 188/182 | 2710 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 186/14 | 375 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | VALIDATION | 186/13 | 364 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | VALIDATION | 186/13 | 364 ms |
| detect-conflict | CONFLICT | JSON | 2048 | VALIDATION | 184/256 | 2837 ms |
| detect-conflict | CONFLICT | JSON | 4096 | VALIDATION | 184/256 | 1611 ms |
| detect-conflict | CONFLICT | JSON | 8192 | VALIDATION | 184/256 | 1686 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | VALIDATION | 169/17 | 389 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | VALIDATION | 169/17 | 378 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | VALIDATION | 169/17 | 700 ms |
| repair-anchor | REPAIR | JSON | 2048 | VALIDATION | 167/185 | 1886 ms |
| repair-anchor | REPAIR | JSON | 4096 | VALIDATION | 167/256 | 1736 ms |
| repair-anchor | REPAIR | JSON | 8192 | VALIDATION | 167/137 | 1239 ms |
