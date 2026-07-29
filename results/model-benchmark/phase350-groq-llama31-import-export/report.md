# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `llama-3.1-8b-instant`
- Runs: 66
- Compiled: 66
- Syntax valid: 13
- Semantically correct: 13
- Input tokens: 3260
- Output tokens: 1341
- Omitted facts: 0
- Errors (compile/provider/validation): 0/49/4
- Rate limited / timed out: 49/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.1-8b-instant" on "cognitive-v2": correct=13/66 syntax_valid=13 (provider_errors=49 validation_errors=4).

### Notes

- `interpret:compiled=66`
- `interpret:errors_compile=0`
- `interpret:errors_provider=49`
- `interpret:errors_validation=4`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.1-8b-instant`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=13`
- `interpret:strongest_format=CHOICE rate=6/18`
- `interpret:syntax_valid=13`
- `interpret:total=66`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=4096 rate=4/22`
- `interpret:weakest_format=JSON rate=0/24`
- `interpret:weakest_operation=CONFLICT rate=0/18`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 18 | 18 | 0 | 0 | 0 | 18 |
| EXTRACT | 18 | 18 | 12 | 12 | 0 | 6 |
| REPAIR | 12 | 12 | 1 | 1 | 0 | 11 |
| SYNTHESIZE | 18 | 18 | 0 | 0 | 0 | 18 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 18 | 18 | 6 | 6 | 0 | 12 |
| DELIMITED | 24 | 24 | 7 | 7 | 0 | 17 |
| JSON | 24 | 24 | 0 | 0 | 0 | 24 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 5 | 5 | 0 | 17 |
| 4096 | 22 | 22 | 4 | 4 | 0 | 18 |
| 8192 | 22 | 22 | 4 | 4 | 0 | 18 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 175/46 | 439 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 175/46 | 355 ms |
| extract-date | EXTRACT | CHOICE | 8192 | CORRECT | 0 | 0 ms | 175/46 | 353 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 173/14 | 214 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 173/14 | 303 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 173/14 | 215 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 172/256 | 536 ms |
| extract-date | EXTRACT | JSON | 4096 | VALIDATION | 0 | 0 ms | 172/256 | 537 ms |
| extract-date | EXTRACT | JSON | 8192 | VALIDATION | 0 | 0 ms | 172/256 | 536 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 220/26 | 241 ms |
| extract-qualified-date | EXTRACT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 220/26 | 288 ms |
| extract-qualified-date | EXTRACT | CHOICE | 8192 | CORRECT | 0 | 0 ms | 220/26 | 250 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 218/14 | 228 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 218/14 | 306 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 218/14 | 218 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 217/256 | 557 ms |
| extract-qualified-date | EXTRACT | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| extract-qualified-date | EXTRACT | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 35 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 36 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 36 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 36 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 36 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | PROVIDER | 429 | 1000 ms | 0/0 | 35 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 37 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 36 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 35 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 36 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 8192 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 4096 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 32 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | PROVIDER | 429 | 1000 ms | 0/0 | 36 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | PROVIDER | 429 | 1000 ms | 0/0 | 33 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 35 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | PROVIDER | 429 | 1000 ms | 0/0 | 35 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | PROVIDER | 429 | 1000 ms | 0/0 | 35 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| detect-conflict | CONFLICT | JSON | 4096 | PROVIDER | 429 | 1000 ms | 0/0 | 35 ms |
| detect-conflict | CONFLICT | JSON | 8192 | PROVIDER | 429 | 1000 ms | 0/0 | 33 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 38 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 4096 | PROVIDER | 429 | 1000 ms | 0/0 | 33 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 8192 | PROVIDER | 429 | 1000 ms | 0/0 | 45 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 32 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 4096 | PROVIDER | 429 | 1000 ms | 0/0 | 34 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 8192 | PROVIDER | 429 | 1000 ms | 0/0 | 35 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | PROVIDER | 429 | 1000 ms | 0/0 | 37 ms |
| distinguish-temporal-change | CONFLICT | JSON | 4096 | PROVIDER | 429 | 1000 ms | 0/0 | 35 ms |
| distinguish-temporal-change | CONFLICT | JSON | 8192 | PROVIDER | 429 | 1000 ms | 0/0 | 36 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 169/17 | 227 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 36 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 35 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 38 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 37 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 36 ms |
| repair-unrecoverable | REPAIR | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| repair-unrecoverable | REPAIR | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 64 ms |
