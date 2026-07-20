# Cognitive campaign

- Name: `continuous-probe-2026-07-20-0235-model-rotation`
- Fixture: `cognitive-v1`
- Planned/max calls: 22/22
- Models: 2

| Provider | Binding | Model | Qualification | Correct | Syntax | Provider errors | 429 | Timeouts | Regressions |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| nvidia_nim | nim-nemotron-nano-9b-v2 | nvidia/nemotron-nano-9b-v2 | INCOMPATIBLE | 0/11 | 0/11 | 11 | 0 | 0 | 0 |
| groq | groq-gpt-oss-20b | openai/gpt-oss-20b | QUALIFIED | 10/11 | 10/11 | 0 | 0 | 0 | 0 |

Qualification is observational evidence only; it does not enable a binding or change runtime routing.

## Regressions
