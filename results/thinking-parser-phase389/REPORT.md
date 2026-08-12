# Phase 389 Live Campaign — Thinking & Structured Response Parsing

Date: 2026-08-12T20:43:48Z

| Model | Tokens | Latency (ms) | Raw Text Preview | StripThinking Applied | DATE Match | SOURCE Match |
|---|---|---|---|---|---|---|
| qwen/qwen3.6-27b | 1024 | 2309 | ` <think> Here's a thinking process:  1.  **Analyze User Inpu...` | strip_unclosed_thinking_tag | false | false |
| groq/compound-mini | 1024 | 1074 | `DATE: 2026-08-08 SOURCE: SYS-ALPHA-99` | none | true | true |
| llama-3.3-70b-versatile | 256 | 317 | `DATE: 2026-08-08 SOURCE: SYS-ALPHA-99` | none | true | true |

**Summary:** Passed 2/3 live calls (66.7%).
