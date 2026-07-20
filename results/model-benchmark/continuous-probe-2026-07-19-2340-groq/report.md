# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `llama-3.1-8b-instant`
- Runs: 22
- Compiled: 22
- Syntax valid: 11
- Semantically correct: 10
- Input tokens: 2810
- Output tokens: 1442
- Omitted facts: 0
- Errors (compile/provider/validation): 0/7/4
- Rate limited / timed out: 7/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.1-8b-instant" on "cognitive-v2": correct=10/22 syntax_valid=11 (provider_errors=7 validation_errors=4).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=7`
- `interpret:errors_validation=4`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.1-8b-instant`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=10`
- `interpret:strongest_format=CHOICE rate=5/6`
- `interpret:syntax_valid=11`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=10/22`
- `interpret:weakest_format=JSON rate=1/8`
- `interpret:weakest_operation=REPAIR rate=0/4`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 2 | 2 | 0 | 4 |
| EXTRACT | 6 | 6 | 4 | 4 | 0 | 2 |
| REPAIR | 4 | 4 | 0 | 0 | 0 | 4 |
| SYNTHESIZE | 6 | 6 | 5 | 4 | 0 | 1 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 5 | 5 | 0 | 1 |
| DELIMITED | 8 | 8 | 5 | 4 | 0 | 3 |
| JSON | 8 | 8 | 1 | 1 | 0 | 7 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 11 | 10 | 0 | 11 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 175/46 | 378 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 173/14 | 221 ms |
| extract-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 172/256 | 563 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 220/26 | 239 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 218/14 | 301 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 217/256 | 557 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 161/9 | 206 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 159/9 | 204 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 0 | 0 ms | 158/256 | 577 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 201/10 | 217 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | INCORRECT | 0 | 0 ms | 199/8 | 313 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 198/256 | 647 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 188/13 | 213 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 186/13 | 221 ms |
| detect-conflict | CONFLICT | JSON | 2048 | VALIDATION | 0 | 0 ms | 185/256 | 548 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | PROVIDER | 429 | 4000 ms | 0/0 | 33 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | PROVIDER | 429 | 5000 ms | 0/0 | 35 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | PROVIDER | 429 | 4000 ms | 0/0 | 34 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 4000 ms | 0/0 | 33 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 429 | 4000 ms | 0/0 | 35 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | PROVIDER | 429 | 4000 ms | 0/0 | 34 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 429 | 2000 ms | 0/0 | 36 ms |
