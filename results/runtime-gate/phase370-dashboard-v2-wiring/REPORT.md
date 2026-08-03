# Phase 370 — live probe bundle: reasoning-eaten budget reproduction + dashboard wiring (2026-08-02)

## Bounds

- 3 live Groq calls (within cycle cap 3); zero canonical state promotion; read-only probes on the composed bootstrap mux.
- Hypothesis: (H1) the dashboard-v2 mux serves shell + embedded assets + vault/inspect endpoints from a single in-memory bootstrap; (H2) Groq `openai/gpt-oss-20b` at `max_tokens=8` reproduces the `reasoning_budget_exhausted` classification introduced after Phase 369 live evidence; (H3) exact-text contracts on `llama-3.1-8b-instant` and `llama-3.3-70b-versatile` remain healthy as rotation control.

## H1 — bootstrap wiring (offline, Go httptest)

All 6 sub-checks PASS:

| check | result |
|---|---|
| `GET /dash/` serves templ shell with `motor-autonomo` marker | 200 |
| `GET /dash/assets/htmx.min.js` embedded asset | 200 |
| `GET /api/vault/health` wired (V2Server.Vault assigned) | 200 JSON with `initialized` or `locked` |
| `GET /api/inspect/health` reachable on shared `/api` base | 200 |

## H2 — live Groq reproduction

### `openai/gpt-oss-20b` at `max_tokens=8`, exact-text prompt

- HTTP 200, 281 ms wall, INVALID_RESPONSE with `reason=reasoning_budget_exhausted`.
- Confirms the Phase 369 secondary evidence: with a tiny output budget the model consumes all 8 tokens in internal reasoning and returns empty semantic content; the adapter now classifies it separately from a genuine `empty_content` failure.
- This is a **token-budget pathology**, not a model refusal — the same prompt succeeds at higher budgets.

### Controls

- `llama-3.1-8b-instant` at `max_tokens=8`, exact-text prompt: HTTP 200, 210 ms, `finish_reason=stop`, text `READY`, 40 input + 2 output tokens.
- `llama-3.3-70b-versatile` at `max_tokens=8`, exact-text prompt: HTTP 200, 289 ms, `finish_reason=stop`, text `READY`, 40 input + 2 output tokens.

## H3 — decision

- New error reason `reasoning_budget_exhausted` is wired through `provider/openai`, covered by regression tests (`reasoning-eaten budget` + negative cases: empty content without reasoning, length finish without reasoning details).
- Callers (gatecampaign, prompt retry policy) can distinguish the two classes and retry with `max_tokens ×4` only in the reasoning-eaten path, per Phase 369 decision.

## Artifacts

- `live-reasoning-repro.json` — 3 sanitized trial records (model, budget, outcome, latency, token usage; no prompts, no credentials).
- `cmd/phase370probe/main.go` — probe binary (not committed; temporary verification harness).

## Verification

- Offline: `go test ./internal/provider/openai/...` ok, `go test ./...` full suite ok, `go vet ./internal/provider/openai/`, `gofmt -l` empty, `git diff --check` clean.
- Live: 3/3 authenticated Groq calls within 45 s deadline, p95 latency 289 ms; zero 429, zero retries; no canonical state promotion.
