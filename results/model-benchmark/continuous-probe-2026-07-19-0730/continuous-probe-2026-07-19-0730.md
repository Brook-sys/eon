# Cognitive campaign

- Name: `continuous-probe-2026-07-19-429`
- Fixture: `cognitive-v2`
- Planned/max calls: 44/44
- Models: 2

| Provider | Binding | Model | Qualification | Correct | Syntax | Provider errors | 429 | Timeouts | Regressions |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| nim | nim-primary | mistralai/mistral-small-4-119b-2603 | INCOMPATIBLE | 0/22 | 0/22 | 22 | 0 | 0 | 0 |
| groq | groq-fallback | llama-3.1-8b-instant | INCOMPATIBLE | 0/22 | 0/22 | 22 | 0 | 0 | 0 |

Qualification is observational evidence only; it does not enable a binding or change runtime routing.

## Regressions

