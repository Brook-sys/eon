# Phase 389 Live Campaign — Thinking & Structured Response Parsing

Date: 2026-08-08T09:24:54Z

| Model | Tokens | Latency (ms) | Raw Text Preview | StripThinking Applied | DATE Match | SOURCE Match |
|---|---|---|---|---|---|---|
| qwen/qwen3.6-27b | 1024 | 2285 | ` <think> Here's a thinking process:  1.  **Analyze User Inpu...` | strip_unclosed_thinking_tag | true | true |
| groq/compound-mini | 1024 | 1082 | `DATE: 2026-08-08 SOURCE: SYS-ALPHA-99` | none | true | true |
| llama-3.3-70b-versatile | 256 | 242 | `DATE: 2026-08-08 SOURCE: SYS-ALPHA-99` | none | true | true |

**Summary:** Passed 3/3 live calls (100.0%).
