# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `llama-3.3-70b-versatile`
- Runs: 66
- Compiled: 66
- Syntax valid: 30
- Semantically correct: 30
- Input tokens: 5562
- Output tokens: 489
- Omitted facts: 0
- Errors (compile/provider/validation): 0/36/0
- Rate limited / timed out: 36/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.3-70b-versatile" on "cognitive-v2": correct=30/66 syntax_valid=30 (provider_errors=36 validation_errors=0).

### Notes

- `interpret:compiled=66`
- `interpret:errors_compile=0`
- `interpret:errors_provider=36`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.3-70b-versatile`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=30`
- `interpret:strongest_format=CHOICE rate=12/18`
- `interpret:syntax_valid=30`
- `interpret:total=66`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=10/22`
- `interpret:weakest_format=DELIMITED rate=9/24`
- `interpret:weakest_operation=CONFLICT rate=0/18`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 18 | 18 | 0 | 0 | 0 | 18 |
| EXTRACT | 18 | 18 | 18 | 18 | 0 | 0 |
| REPAIR | 12 | 12 | 0 | 0 | 0 | 12 |
| SYNTHESIZE | 18 | 18 | 12 | 12 | 0 | 6 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 18 | 18 | 12 | 12 | 0 | 6 |
| DELIMITED | 24 | 24 | 9 | 9 | 0 | 15 |
| JSON | 24 | 24 | 9 | 9 | 0 | 15 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 10 | 10 | 0 | 12 |
| 4096 | 22 | 22 | 10 | 10 | 0 | 12 |
| 8192 | 22 | 22 | 10 | 10 | 0 | 12 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 175/14 | 504 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 175/14 | 321 ms |
| extract-date | EXTRACT | CHOICE | 8192 | CORRECT | 0 | 0 ms | 175/14 | 315 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 173/14 | 258 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 173/14 | 309 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 173/14 | 254 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 172/28 | 365 ms |
| extract-date | EXTRACT | JSON | 4096 | CORRECT | 0 | 0 ms | 172/28 | 237 ms |
| extract-date | EXTRACT | JSON | 8192 | CORRECT | 0 | 0 ms | 172/28 | 327 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 220/14 | 313 ms |
| extract-qualified-date | EXTRACT | CHOICE | 4096 | CORRECT | 0 | 0 ms | 220/14 | 338 ms |
| extract-qualified-date | EXTRACT | CHOICE | 8192 | CORRECT | 0 | 0 ms | 220/14 | 341 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 218/14 | 349 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 218/14 | 331 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 218/14 | 229 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | CORRECT | 0 | 0 ms | 217/28 | 349 ms |
| extract-qualified-date | EXTRACT | JSON | 4096 | CORRECT | 0 | 0 ms | 217/28 | 342 ms |
| extract-qualified-date | EXTRACT | JSON | 8192 | CORRECT | 0 | 0 ms | 217/28 | 326 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 161/9 | 319 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 0 | 0 ms | 161/9 | 310 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 0 | 0 ms | 161/9 | 212 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 159/9 | 303 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 0 | 0 ms | 159/9 | 299 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 0 | 0 ms | 159/9 | 299 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 158/23 | 327 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | CORRECT | 0 | 0 ms | 158/23 | 319 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | CORRECT | 0 | 0 ms | 158/23 | 326 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 201/10 | 359 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 4096 | CORRECT | 0 | 0 ms | 201/10 | 367 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 8192 | CORRECT | 0 | 0 ms | 201/10 | 487 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 32 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 35 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 31 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 32 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 32 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 31 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 31 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 31 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 32 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 31 ms |
| detect-conflict | CONFLICT | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 32 ms |
| detect-conflict | CONFLICT | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 31 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 30 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| distinguish-temporal-change | CONFLICT | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 37 ms |
| distinguish-temporal-change | CONFLICT | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 32 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 34 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 32 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 31 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 32 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 31 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 43 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 48 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 43 ms |
| repair-unrecoverable | REPAIR | JSON | 4096 | PROVIDER | 429 | 2000 ms | 0/0 | 40 ms |
| repair-unrecoverable | REPAIR | JSON | 8192 | PROVIDER | 429 | 2000 ms | 0/0 | 33 ms |
