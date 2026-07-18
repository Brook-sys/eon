# Live model qualification summary — 2026-07-18

| Campaign | Model | Correct | Provider errors | Validation errors | Input tokens | Output tokens |
|---|---|---:|---:|---:|---:|---:|
| `groq-gpt-oss-20b-contract-2026-07-18` | `openai/gpt-oss-20b` | 19/33 | 14 | 0 | 3873 | 1967 |
| `new-candidates-2026-07-18` | `openai/gpt-oss-120b` | 10/11 | 1 | 0 | 2071 | 1313 |
| `groq-qwen3.6-27b-contract-2026-07-18` | `qwen/qwen3.6-27b` | 0/33 | 11 | 22 | 3465 | 5632 |
| `live-groq-llama-3.1-8b-contract-2026-07-18` | `llama-3.1-8b-instant` | 12/33 | 18 | 3 | 2520 | 1002 |
| `live-groq-llama-3.3-70b-contract-2026-07-18` | `llama-3.3-70b-versatile` | 24/33 | 3 | 6 | 5178 | 521 |
| `live-nvidia-llama-3.1-8b-contract-2026-07-18` | `meta/llama-3.1-8b-instruct` | 15/33 | 0 | 18 | 5670 | 4282 |
| `nvidia-nemotron-3-nano-30b-a3b-contract-2026-07-18` | `nvidia/nemotron-3-nano-30b-a3b` | 16/33 | 0 | 17 | 5637 | 7495 |
| `new-candidates-2026-07-18` | `mistralai/mistral-small-4-119b-2603` | 9/11 | 0 | 2 | 1762 | 205 |
| `nvidia-qwen3-next-80b-a3b-contract-2026-07-18` | `qwen/qwen3-next-80b-a3b-instruct` | 0/33 | 33 | 0 | 0 | 0 |

The campaign classifier marks NVIDIA Mistral Small 4 as `QUALIFIED` for this
11-call 2k slice (≥2/3 correct and no provider failures). Groq GPT-OSS 120B is
`DEGRADED`, despite 10/11 correct, because one provider failure prevents the
strict qualified verdict. The prior 33-call results classify GPT-OSS 20B,
Nemotron Nano 30B and both Llama 3.1 8B deployments as `DEGRADED`; Groq Llama
3.3 70B as `QUALIFIED`; and Qwen 3.6 / Qwen3 Next as `INCOMPATIBLE` in the
observed deployments.

Results are observational evidence only. They do not automatically change runtime routing or grant model output authority.
