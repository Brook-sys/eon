# Cognitive benchmark

- Fixture: `cognitive-tool-v1`
- Model: `offline-oracle`
- Runs: 3
- Compiled: 3
- Syntax valid: 3
- Semantically correct: 3
- Input tokens: 96
- Output tokens: 6
- Omitted facts: 0
- Errors (compile/provider/validation): 0/0/0
- Rate limited / timed out: 0/0

## Interpretation

- Kind: `offline-oracle`
- Verdict: `PASS`
- Headline: Offline oracle PASS on "cognitive-tool-v1": 3/3 runs semantically correct (encode→Parse ceiling; not a live model skill).

### Notes

- `interpret:compiled=3`
- `interpret:encode_parse_roundtrip_ok`
- `interpret:errors_compile=0`
- `interpret:errors_provider=0`
- `interpret:errors_validation=0`
- `interpret:fixture=cognitive-tool-v1`
- `interpret:kind=offline-oracle`
- `interpret:model=offline-oracle`
- `interpret:oracle_is_harness_ceiling_not_model_skill`
- `interpret:semantically_correct=3`
- `interpret:strongest_format=JSON rate=3/3`
- `interpret:syntax_valid=3`
- `interpret:total=3`
- `interpret:verdict=PASS`
- `interpret:weakest_context=2048 rate=1/1`
- `interpret:weakest_format=JSON rate=3/3`
- `interpret:weakest_operation=SYNTHESIZE rate=3/3`

## By operation

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| SYNTHESIZE | 3 | 3 | 3 | 3 | 0 | 0 |

## By format

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| JSON | 3 | 3 | 3 | 3 | 0 | 0 |

## By context

| Group | Runs | Compiled | Syntax valid | Correct | Omitted facts | Errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 2048 | 1 | 1 | 1 | 1 | 0 | 0 |
| 4096 | 1 | 1 | 1 | 1 | 0 | 0 |
| 8192 | 1 | 1 | 1 | 1 | 0 | 0 |

## Runs

| Case | Operation | Format | Context | Result | HTTP | Retry-After | Input/output tokens | Latency |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| tool-search-single | SYNTHESIZE | JSON | 2048 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| tool-search-single | SYNTHESIZE | JSON | 4096 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
| tool-search-single | SYNTHESIZE | JSON | 8192 | CORRECT | 0 | 0 ms | 32/2 | 0 ms |
