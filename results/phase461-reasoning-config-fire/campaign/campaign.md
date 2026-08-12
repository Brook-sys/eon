# Cognitive campaign

- Name: `phase461-reasoning-config-fire`
- Fixture: `cognitive-v2`
- Planned/max calls: 132/150
- Models: 6

| Provider | Binding | Model | Qualification | Correct | Syntax | Provider errors | 429 | Timeouts | Regressions |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| openai | groq-llama-3.1-8b-instant-baseline | llama-3.1-8b-instant | INCOMPATIBLE | 5/22 | 5/22 | 15 | 15 | 0 | 0 |
| openai | groq-gpt-oss-20b-effort-low-parsed | openai/gpt-oss-20b | DEGRADED | 20/22 | 20/22 | 1 | 0 | 0 | 0 |
| openai | groq-gpt-oss-20b-effort-medium-parsed | openai/gpt-oss-20b | INCOMPATIBLE | 0/22 | 0/22 | 22 | 22 | 0 | 0 |
| openai | groq-qwen3.6-27b-effort-none-parsed | qwen/qwen3.6-27b | INCOMPATIBLE | 0/22 | 0/22 | 11 | 11 | 0 | 0 |
| openai | groq-qwen3.6-27b-effort-default-parsed | qwen/qwen3.6-27b | INCOMPATIBLE | 0/22 | 0/22 | 21 | 21 | 0 | 0 |
| openai | groq-gpt-oss-120b-effort-low-parsed | openai/gpt-oss-120b | DEGRADED | 13/22 | 13/22 | 9 | 9 | 0 | 0 |

Qualification is observational evidence only; it does not enable a binding or change runtime routing.

## Regressions

