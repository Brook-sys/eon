# Dashboard Assets

Build pipeline for the v2 dashboard (Templ + HTMX + Tailwind + Alpine.js).

## Prerequisites

1. **templ CLI** — `go install github.com/a-h/templ/cmd/templ@latest`
2. **Tailwind standalone CLI** — download from [GitHub releases](https://github.com/tailwindlabs/tailwindcss/releases) and place at `tools/dashboard-assets/tailwindcss` (chmod +x). Not committed (107 MB binary).

## Build

```bash
./tools/dashboard-assets/build.sh
```

This will:
1. Compile Tailwind CSS from `internal/dashboard/assets/src/app.css` → `internal/dashboard/assets/dist/app.css`
2. Copy `htmx.min.js` and `alpine.min.js` to `internal/dashboard/assets/dist/`
3. Run `templ generate` to produce `*_templ.go` files

## Runtime

Assets are embedded via `go:embed` in `internal/dashboard/assets/assets.go`. Zero runtime file dependencies.

## V2 Dashboard Routes

- `GET /dash/` — Overview page (Templ)
- `GET /dash/assets/*` — Static assets (HTMX, Alpine, CSS)
- `GET /dash/events` — HTMX partial (Phase 2)

Legacy dashboard at `/` and `/dashboard` remains untouched.
