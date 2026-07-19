# Blocked: Missing API Keys

Attempted to run the continuous-probe against Groq and NVIDIA NIM, but the API keys are not present in this runtime (HTTP 401 Unauthorized for both).
The environment variables NVIDIA_NIM_API_KEY and GROQ_API_KEY are not set.

This blocks the task "avaliar reload atômico MODELS no boundary de ciclo" since we cannot establish the baseline live capability without authentication.
