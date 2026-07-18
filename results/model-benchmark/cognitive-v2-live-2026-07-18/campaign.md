# Cognitive campaign

- Name: `cognitive-v2-live-2026-07-18`
- Fixture: `cognitive-v2`
- Planned/max calls: 44/44
- Models: 2

| Provider | Binding | Model | Correct | Syntax | Provider errors | 429 | Timeouts | Regressions |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| groq | groq-llama-3.3-70b | llama-3.3-70b-versatile | 19/22 | 19/22 | 0 | 0 | 0 | 0 |
| nvidia_nim | nvidia-llama-3.1-8b | meta/llama-3.1-8b-instruct | 11/22 | 11/22 | 5 | 0 | 0 | 6 |

## Regressions

- `nvidia-llama-3.1-8b` format/DELIMITED semantically_correct: 11/12 → 7/8
- `nvidia-llama-3.1-8b` format/DELIMITED syntax_valid: 11/12 → 7/8
- `nvidia-llama-3.1-8b` format/JSON semantically_correct: 1/12 → 0/8
- `nvidia-llama-3.1-8b` format/JSON syntax_valid: 1/12 → 0/8
- `nvidia-llama-3.1-8b` operation/REPAIR semantically_correct: 4/6 → 1/4
- `nvidia-llama-3.1-8b` operation/REPAIR syntax_valid: 4/6 → 1/4
