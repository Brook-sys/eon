# Phase 530-533 - Model Availability & Deprecation Status (2026-08-14)

**Objective**: Verify model status of newly surfaced large models and document availability changes across Groq and NIM deployments.

## Campaigns & Results

### Phase 530 - Groq DeepSeek Deprecation
- **Tested Deployment**: `deepseek-r1-distill-llama-70b` on Groq.
- **Result**: `HTTP 400 Bad Request`.
- **Finding**: Groq API explicitly rejected the call, responding that the model `deepseek-r1-distill-llama-70b` has been decommissioned (`code: "model_decommissioned"`). 
- **Action**: Do not use `deepseek-r1-distill-llama-70b` on Groq.

### Phase 531 - NIM DeepSeek V4 Flash
- **Tested Deployment**: `deepseek-ai/deepseek-v4-flash-0731` on NVIDIA NIM.
- **Result**: `OK` (Format Compliance 1.0).
- **Latency**: 15.45 seconds.
- **Finding**: High latency on completion (15s for 15 output tokens), but successfully executes the structured format prompt natively. 

### Phase 532 - NIM Meta Llama 3.3 70B
- **Tested Deployment**: `meta/llama-3.3-70b-instruct` on NVIDIA NIM.
- **Result**: `TRANSPORT` timeout (120 seconds).
- **Tested Deployment 2**: `nvidia/llama-3.1-nemotron-70b-instruct` on NVIDIA NIM.
- **Result**: `HTTP 404`.
- **Tested Deployment 3**: `nvidia/llama-3.3-nemotron-super-49b-v1.5` on NVIDIA NIM.
- **Result**: `INVALID_RESPONSE: empty_content`.
- **Finding**: High failure rate across NIM 70B class models. We will stick to Meta Llama 3.1 8B Instruct for NIM fallback.

### Phase 533 - Groq Meta Llama 3.3 70B
- **Tested Deployment**: `llama-3.3-70b-versatile` on Groq.
- **Result**: `OK` (Format Compliance 1.0).
- **Latency**: 378 ms.
- **Finding**: Exceptional performance. 116 input tokens and 14 output tokens in 378ms.

## Decisions
- Update CONTINUOUS_DEVELOPMENT.md to record the decommissioning of DeepSeek on Groq and the severe degradation/timeouts on NIM's 70B class models.
- Maintain `llama-3.3-70b-versatile` on Groq as the primary large context workhorse.
- Retain `deepseek-ai/deepseek-v4-flash-0731` on NIM purely for functional cross-provider testing, keeping in mind the heavy latency.
