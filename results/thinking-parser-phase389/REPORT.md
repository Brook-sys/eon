# Phase 389 Live Campaign — Thinking & Structured Response Parsing

Date: 2026-08-12T20:42:42Z

| Model | Tokens | Latency (ms) | Raw Text Preview | StripThinking Applied | DATE Match | SOURCE Match |
|---|---|---|---|---|---|---|
| qwen/qwen3.6-27b | 1024 | 2281 | ` <think> Here's a thinking process:  1.  **Analyze User Inpu...` | strip_thinking_tags | true | true |
| groq/compound-mini | 1024 | 906 | `DATE: 2026-08-08 SOURCE: SYS-ALPHA-99` | none | true | true |
| llama-3.3-70b-versatile | 256 | 628 | `DATE: 2026-08-08 SOURCE: SYS-ALPHA-99` | none | true | true |

**Summary:** Passed 3/3 live calls (100.0%).
