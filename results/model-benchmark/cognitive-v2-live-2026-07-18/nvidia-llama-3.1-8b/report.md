# Cognitive benchmark

- Fixture: `cognitive-v2`
- Model: `meta/llama-3.1-8b-instruct`
- Runs: 22
- Compiled: 22
- Syntax valid: 11
- Semantically correct: 11
- Input tokens: 3182
- Output tokens: 1547
- Omitted facts: 0
- Errors (compile/provider/validation): 0/5/6
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "meta/llama-3.1-8b-instruct" on "cognitive-v2": correct=11/22 syntax_valid=11 (provider_errors=5 validation_errors=6).

### Notes

- `interpret:compiled=22`
- `interpret:errors_compile=0`
- `interpret:errors_provider=5`
- `interpret:errors_validation=6`
- `interpret:fixture=cognitive-v2`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=meta/llama-3.1-8b-instruct`
- `interpret:prefer_empirically_stronger_format_or_smaller_ops_first`
- `interpret:semantically_correct=11`
- `interpret:strongest_format=DELIMITED rate=7/8`
- `interpret:syntax_valid=11`
- `interpret:total=22`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=11/22`
- `interpret:weakest_format=JSON rate=0/8`
- `interpret:weakest_operation=REPAIR rate=1/4`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 6 | 6 | 3 | 3 | 0 | 3 |
| EXTRACT | 6 | 6 | 3 | 3 | 0 | 3 |
| REPAIR | 4 | 4 | 1 | 1 | 0 | 3 |
| SYNTHESIZE | 6 | 6 | 4 | 4 | 0 | 2 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 6 | 6 | 4 | 4 | 0 | 2 |
| DELIMITED | 8 | 8 | 7 | 7 | 0 | 1 |
| JSON | 8 | 8 | 0 | 0 | 0 | 8 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 22 | 22 | 11 | 11 | 0 | 11 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 175/226 | 2324 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 173/14 | 483 ms |
| extract-date | EXTRACT | JSON | 2048 | PROVIDER | 0 | 0 ms | 0/0 | 90000 ms |
| extract-qualified-date | EXTRACT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 220/26 | 657 ms |
| extract-qualified-date | EXTRACT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 218/14 | 487 ms |
| extract-qualified-date | EXTRACT | JSON | 2048 | VALIDATION | 0 | 0 ms | 216/231 | 3029 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 161/9 | 358 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 159/9 | 382 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | VALIDATION | 0 | 0 ms | 157/256 | 3052 ms |
| synthesize-counterexample | SYNTHESIZE | CHOICE | 2048 | CORRECT | 0 | 0 ms | 201/10 | 453 ms |
| synthesize-counterexample | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 199/10 | 577 ms |
| synthesize-counterexample | SYNTHESIZE | JSON | 2048 | VALIDATION | 0 | 0 ms | 197/249 | 1908 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 0 | 0 ms | 188/224 | 3001 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 186/13 | 61559 ms |
| detect-conflict | CONFLICT | JSON | 2048 | PROVIDER | 503 | 0 ms | 0/0 | 299 ms |
| distinguish-temporal-change | CONFLICT | CHOICE | 2048 | CORRECT | 0 | 0 ms | 199/14 | 380 ms |
| distinguish-temporal-change | CONFLICT | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 197/14 | 386 ms |
| distinguish-temporal-change | CONFLICT | JSON | 2048 | PROVIDER | 503 | 0 ms | 0/0 | 289 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 0 | 0 ms | 169/17 | 496 ms |
| repair-anchor | REPAIR | JSON | 2048 | VALIDATION | 0 | 0 ms | 167/211 | 1275 ms |
| repair-unrecoverable | REPAIR | DELIMITED | 2048 | PROVIDER | 503 | 0 ms | 0/0 | 302 ms |
| repair-unrecoverable | REPAIR | JSON | 2048 | PROVIDER | 503 | 0 ms | 0/0 | 286 ms |
