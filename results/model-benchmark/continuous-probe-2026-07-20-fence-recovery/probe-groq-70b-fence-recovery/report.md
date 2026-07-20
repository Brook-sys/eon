# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `llama-3.3-70b-versatile`
- Runs: 22
- Compiled: 22
- Syntax valid: 19
- Semantically correct: 19
- Input tokens: 4122
- Output tokens: 578
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/3
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.3-70b-versatile" on "cognitive-v2": correct=19/22 syntax_valid=19 (provider_errors=0 validation_errors=3).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=3`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.3-70b-versatile`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=19`
- `interpret:strongest_format=JSON rate=8/8`
- `interpret:syntax_valid=19`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=4096 rate=19/22`
- `interpret:weakest_format=DELIMITED rate=6/8`
- `interpret:weakest_operation=CONFLICT rate=4/6`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 4 | 4 | 0 | 2 |
| EXTRACT | 6 | 6 | 6 | 6 | 0 | 0 |
| REPAIR | 4 | 4 | 3 | 3 | 0 | 1 |
| SYNTHESIZE | 6 | 6 | 6 | 6 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 5 | 5 | 0 | 1 |
| DELIMITED | 8 | 8 | 6 | 6 | 0 | 2 |
| JSON | 8 | 8 | 8 | 8 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 4096 | 22 | 22 | 19 | 19 | 0 | 3 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 175/14 | 384 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 173/14 | 253 ms |
| extract-date | EXTRACT | JSON | 4096 | CORRECT | 0 | 0 ms | 172/28 | 278 ms |
| extract-qualified-date | EXTRACT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 220/14 | 416 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 218/14 | 375 ms |
| extract-qualified-date | EXTRACT | JSON | 4096 | CORRECT | 0 | 0 ms | 217/28 | 384 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 0 | 0 ms | 161/9 | 318 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 159/9 | 234 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | CORRECT | 0 | 0 ms | 158/23 | 256 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 4096 | CORRECT | 0 | 0 ms | 201/10 | 307 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 199/10 | 377 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 4096 | CORRECT | 0 | 0 ms | 198/23 | 356 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | VALIDATION | 0 | 0 ms | 188/17 | 362 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | VALIDATION | 0 | 0 ms | 186/13 | 238 ms |
| detect-conflict | CONFLICT | JSON | 4096 | CORRECT | 0 | 0 ms | 185/27 | 268 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 199/14 | 346 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 197/14 | 352 ms |
| distinguish-temporal-change | CONFLICT | JSON | 4096 | CORRECT | 0 | 0 ms | 196/27 | 339 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 169/17 | 223 ms |
| repair-anchor | REPAIR | JSON | 4096 | CORRECT | 0 | 0 ms | 168/30 | 372 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 4096 | VALIDATION | 0 | 0 ms | 192/11 | 347 ms |
| repair-unrecoverable | REPAIR | JSON | 4096 | CORRECT | 0 | 0 ms | 191/212 | 975 ms |
