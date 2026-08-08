# Phase 394 — Format Anchoring Fire Test Report

**Date:** 2026-08-08 13:52:27
**Total Trials:** 36
**Semantic Success Rate:** 75.0% (27/36)
**Avg Format Compliance:** 75.0%
**P50 Latency:** 729 ms
**P95 Latency:** 2695 ms

---

## Key Observations

1. **Format Anchoring Effectiveness:** Appending explicit FORMAT RULE blocks under tight output token budgets (max_tokens=64) significantly increases primary format compliance across 8B models.
2. **Parser Strategy Telemetry:** The new ParseStrategy telemetry cleanly distinguishes primary_prefix parsing from positional_fallback and unparsed responses.
3. **Multi-Model Behavior:** Tested across Groq (llama-3.1-8b-instant, llama-3.3-70b-versatile, qwen/qwen3.6-27b) and NVIDIA NIM (deepseek-v4-flash-0731).

Artifacts saved to results/phase394-format-anchoring/.
