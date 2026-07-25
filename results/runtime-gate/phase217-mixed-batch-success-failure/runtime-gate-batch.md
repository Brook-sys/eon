# Runtime provider gate batch

- Name: `phase217-mixed-batch-success-failure`
- Trials/calls: 2/2
- Provider successes: 2
- Execution failures: 2
- Durable reopens: 2
- Expected matches: 0
- JSON valid: 0
- Tokens input/output: 1206/768
- Provider latency p50/p95/max: `1.064717533s` / `1.115632743s` / `1.115632743s`
- Selected bindings: `map[groq-qwen36-27b:2]`
- Finish reasons: `map[length:2]`
- Framing classes: `map[invalid_json:2]`
- Second acquire reasons: `map[:2]`

Each trial used a fresh SQLite store and retained the one-external-call ceiling. The aggregate has no authority to alter model routing.
