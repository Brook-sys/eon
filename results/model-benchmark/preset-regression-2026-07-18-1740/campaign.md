# Cognitive campaign

- Name: `preset-regression-2026-07-18-1740`
- Fixture: `cognitive-v1`
- Planned/max calls: 11/11
- Models: 1

| Provider | Binding | Model | Qualification | Correct | Syntax | Provider errors | 429 | Timeouts | Regressions |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| groq | groq-llama-3.3-70b-preset | llama-3.3-70b-versatile | INCOMPATIBLE | 0/11 | 0/11 | 11 | 0 | 0 | 16 |

Qualification is observational evidence only; it does not enable a binding or change runtime routing.

## Regressions

- `groq-llama-3.3-70b-preset` context/2048 semantically_correct: 8/11 → 0/11
- `groq-llama-3.3-70b-preset` context/2048 syntax_valid: 8/11 → 0/11
- `groq-llama-3.3-70b-preset` format/CHOICE semantically_correct: 6/9 → 0/3
- `groq-llama-3.3-70b-preset` format/CHOICE syntax_valid: 6/9 → 0/3
- `groq-llama-3.3-70b-preset` format/DELIMITED semantically_correct: 9/12 → 0/4
- `groq-llama-3.3-70b-preset` format/DELIMITED syntax_valid: 9/12 → 0/4
- `groq-llama-3.3-70b-preset` format/JSON semantically_correct: 9/12 → 0/4
- `groq-llama-3.3-70b-preset` format/JSON syntax_valid: 9/12 → 0/4
- `groq-llama-3.3-70b-preset` operation/CONFLICT semantically_correct: 3/9 → 0/3
- `groq-llama-3.3-70b-preset` operation/CONFLICT syntax_valid: 3/9 → 0/3
- `groq-llama-3.3-70b-preset` operation/EXTRACT semantically_correct: 9/9 → 0/3
- `groq-llama-3.3-70b-preset` operation/EXTRACT syntax_valid: 9/9 → 0/3
- `groq-llama-3.3-70b-preset` operation/REPAIR semantically_correct: 3/6 → 0/2
- `groq-llama-3.3-70b-preset` operation/REPAIR syntax_valid: 3/6 → 0/2
- `groq-llama-3.3-70b-preset` operation/SYNTHESIZE semantically_correct: 9/9 → 0/3
- `groq-llama-3.3-70b-preset` operation/SYNTHESIZE syntax_valid: 9/9 → 0/3
