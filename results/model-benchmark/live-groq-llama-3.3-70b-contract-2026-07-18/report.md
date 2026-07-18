# Cognitive benchmark

- Fixture: `cognitive-v1`
- Model: `llama-3.3-70b-versatile`
- Runs: 33
- Compiled: 33
- Syntax valid: 24
- Semantically correct: 24
- Input tokens: 5178
- Output tokens: 521
- Omitted facts: 0
- Errors (compile/provider/validation): 0/3/6

## Interpretation

- Kind: `live`
- Verdict: `PARTIAL`
- Headline: Cognitive baseline PARTIAL for model "llama-3.3-70b-versatile" on "cognitive-v1": correct=24/33 syntax_valid=24 (provider_errors=3 validation_errors=6).

### Notes

- `interpret:compiled=33`
- `interpret:errors_compile=0`
- `interpret:errors_provider=3`
- `interpret:errors_validation=6`
- `interpret:fixture=cognitive-v1`
- `interpret:kind=live`
- `interpret:live_provider_baseline`
- `interpret:model=llama-3.3-70b-versatile`
- `interpret:prefer_weaker_formats_or_smaller_ops_first`
- `interpret:semantically_correct=24`
- `interpret:syntax_valid=24`
- `interpret:total=33`
- `interpret:verdict=PARTIAL`
- `interpret:weakest_context=2048 rate=8/11`
- `interpret:weakest_format=CHOICE rate=6/9`
- `interpret:weakest_operation=CONFLICT rate=3/9`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CONFLICT | 9 | 9 | 3 | 3 | 0 | 6 |
| EXTRACT | 9 | 9 | 9 | 9 | 0 | 0 |
| REPAIR | 6 | 6 | 3 | 3 | 0 | 3 |
| SYNTHESIZE | 9 | 9 | 9 | 9 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CHOICE | 9 | 9 | 6 | 6 | 0 | 3 |
| DELIMITED | 12 | 12 | 9 | 9 | 0 | 3 |
| JSON | 12 | 12 | 9 | 9 | 0 | 3 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 11 | 11 | 8 | 8 | 0 | 3 |
| 4096 | 11 | 11 | 8 | 8 | 0 | 3 |
| 8192 | 11 | 11 | 8 | 8 | 0 | 3 |

## Runs

| Case | Operation | Format | Context | Result | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: |
| extract-date | EXTRACT | CHOICE | 2048 | CORRECT | 175/14 | 292 ms |
| extract-date | EXTRACT | CHOICE | 4096 | CORRECT | 175/14 | 233 ms |
| extract-date | EXTRACT | CHOICE | 8192 | CORRECT | 175/14 | 320 ms |
| extract-date | EXTRACT | DELIMITED | 2048 | CORRECT | 173/14 | 315 ms |
| extract-date | EXTRACT | DELIMITED | 4096 | CORRECT | 173/14 | 309 ms |
| extract-date | EXTRACT | DELIMITED | 8192 | CORRECT | 173/14 | 247 ms |
| extract-date | EXTRACT | JSON | 2048 | CORRECT | 172/28 | 327 ms |
| extract-date | EXTRACT | JSON | 4096 | CORRECT | 172/28 | 239 ms |
| extract-date | EXTRACT | JSON | 8192 | CORRECT | 172/28 | 243 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 2048 | CORRECT | 161/9 | 215 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 4096 | CORRECT | 161/9 | 220 ms |
| synthesize-support | SYNTHESIZE | CHOICE | 8192 | CORRECT | 161/9 | 219 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 2048 | CORRECT | 159/9 | 304 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 4096 | CORRECT | 159/9 | 300 ms |
| synthesize-support | SYNTHESIZE | DELIMITED | 8192 | CORRECT | 159/9 | 206 ms |
| synthesize-support | SYNTHESIZE | JSON | 2048 | CORRECT | 158/23 | 251 ms |
| synthesize-support | SYNTHESIZE | JSON | 4096 | CORRECT | 158/23 | 341 ms |
| synthesize-support | SYNTHESIZE | JSON | 8192 | CORRECT | 158/23 | 252 ms |
| detect-conflict | CONFLICT | CHOICE | 2048 | VALIDATION | 188/17 | 323 ms |
| detect-conflict | CONFLICT | CHOICE | 4096 | VALIDATION | 188/17 | 228 ms |
| detect-conflict | CONFLICT | CHOICE | 8192 | VALIDATION | 188/17 | 314 ms |
| detect-conflict | CONFLICT | DELIMITED | 2048 | VALIDATION | 186/17 | 312 ms |
| detect-conflict | CONFLICT | DELIMITED | 4096 | VALIDATION | 186/13 | 232 ms |
| detect-conflict | CONFLICT | DELIMITED | 8192 | VALIDATION | 186/17 | 223 ms |
| detect-conflict | CONFLICT | JSON | 2048 | CORRECT | 185/27 | 255 ms |
| detect-conflict | CONFLICT | JSON | 4096 | CORRECT | 185/27 | 349 ms |
| detect-conflict | CONFLICT | JSON | 8192 | CORRECT | 185/27 | 368 ms |
| repair-anchor | REPAIR | DELIMITED | 2048 | CORRECT | 169/17 | 307 ms |
| repair-anchor | REPAIR | DELIMITED | 4096 | CORRECT | 169/17 | 323 ms |
| repair-anchor | REPAIR | DELIMITED | 8192 | CORRECT | 169/17 | 315 ms |
| repair-anchor | REPAIR | JSON | 2048 | PROVIDER | 0/0 | 33 ms |
| repair-anchor | REPAIR | JSON | 4096 | PROVIDER | 0/0 | 33 ms |
| repair-anchor | REPAIR | JSON | 8192 | PROVIDER | 0/0 | 32 ms |
