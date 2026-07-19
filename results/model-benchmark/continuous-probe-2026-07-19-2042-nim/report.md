# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `meta/llama-3.1-8b-instruct`
- Runs: 22
- Compiled: 22
- Syntax valid: 13
- Semantically correct: 13
- Input tokens: 4114
- Output tokens: 2305
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/9
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "meta/llama-3.1-8b-instruct" on "cognitive-v2": correct=13/22 syntax_valid=13 (provider_errors=0 validation_errors=9).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=9`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=meta/llama-3.1-8b-instruct`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=13`
- `interpret:strongest_format=DELIMITED rate=7/8`
- `interpret:syntax_valid=13`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=13/22`
- `interpret:weakest_format=JSON rate=2/8`
- `interpret:weakest_operation=CONFLICT rate=2/6`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 2 | 2 | 0 | 4 |
| EXTRACT | 6 | 6 | 3 | 3 | 0 | 3 |
| REPAIR | 4 | 4 | 4 | 4 | 0 | 0 |
| SYNTHESIZE | 6 | 6 | 4 | 4 | 0 | 2 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 4 | 4 | 0 | 2 |
| DELIMITED | 8 | 8 | 7 | 7 | 0 | 1 |
| JSON | 8 | 8 | 2 | 2 | 0 | 6 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 13 | 13 | 0 | 9 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 175/47 | 988 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 173/14 | 485 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 171/211 | 1859 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 220/193 | 1976 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 218/14 | 700 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 216/246 | 2773 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 161/9 | 415 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 159/9 | 372 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 0 | 0 ms | 157/237 | 2324 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 201/10 | 384 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 199/10 | 403 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | VALIDATION | 0 | 0 ms | 197/256 | 2230 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 188/124 | 1259 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | VALIDATION | 0 | 0 ms | 186/17 | 548 ms |
| detect-conflict | CONFLICT | JSON | 2048 | VALIDATION | 0 | 0 ms | 184/256 | 2529 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 199/14 | 418 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 197/14 | 486 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | VALIDATION | 0 | 0 ms | 195/256 | 2289 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 169/17 | 499 ms |
| repair-anchor | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 167/84 | 1273 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 192/11 | 417 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | CORRECT | 0 | 0 ms | 190/256 | 3090 ms |
