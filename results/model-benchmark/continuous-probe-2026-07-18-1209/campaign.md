# Cognitive campaign

- Name: `continuous-probe-2026-07-18-1209`
- Fixture: `cognitive-v1`
- Planned/max calls: 22/22
- Models: 2

| Provider | Binding | Model | Qualification | Correct | Syntax | Provider errors | 429 | Timeouts | Regressions |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| groq | groq-compound-mini | groq/compound-mini | INCOMPATIBLE | 4/11 | 4/11 | 7 | 7 | 0 | 0 |
| nvidia_nim | nvidia-nemotron-super-49b-v1-5 | nvidia/llama-3.3-nemotron-super-49b-v1.5 | INCOMPATIBLE | 0/11 | 0/11 | 11 | 0 | 0 | 0 |

Qualification is observational evidence only; it does not enable a binding or change runtime routing.

## Regressions
