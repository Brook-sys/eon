# Cognitive campaign

- Name: `new-candidates-2026-07-18`
- Fixture: `cognitive-v1`
- Planned/max calls: 22/22
- Models: 2

| Provider | Binding | Model | Qualification | Correct | Syntax | Provider errors | 429 | Timeouts | Regressions |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| groq | groq-gpt-oss-120b | openai/gpt-oss-120b | DEGRADED | 10/11 | 10/11 | 1 | 0 | 0 | 0 |
| nvidia_nim | nvidia-mistral-small-4-119b | mistralai/mistral-small-4-119b-2603 | QUALIFIED | 9/11 | 9/11 | 0 | 0 | 0 | 0 |

Qualification is observational evidence only; it does not enable a binding or change runtime routing.

## Regressions
