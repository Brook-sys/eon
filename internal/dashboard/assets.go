package dashboard

import (
	"net/http"

	"motor-autonomo/internal/dashboard/assets"
)

// assetsHandler serves the compiled dashboard assets.
// Uses views.Exported* because assets.go and assetsServer both live in package views.
type assetsHandler struct{}

func (assetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/htmx.min.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		w.Write(assets.Htmx2Bytes)
	case r.URL.Path == "/alpine.min.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		w.Write(assets.AlpineBytes)
	case r.URL.Path == "/app.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		w.Write(assets.CSSBytes)
	default:
		http.NotFound(w, r)
	}
}
