package dashboard

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"motor-autonomo/internal/dashboard/views"
)

// V2Server serves the Templ-based dashboard pages under /dash.
// It composes with the existing Server for legacy /api routes.
type V2Server struct {
	// Inspect/Control are the same handlers used by the existing dashboard.
	Inspect http.Handler
	Control http.Handler
	// Vault is an optional localhost-only write-only credential surface.
	Vault http.Handler
	// APIBase is the browser-visible prefix for fetch/EventSource calls.
	APIBase string
	// Logger receives dashboard lifecycle messages.
	Logger *log.Logger
}

func NewV2(inspectAPI, controlAPI http.Handler, logger *log.Logger) (*V2Server, error) {
	if inspectAPI == nil || controlAPI == nil {
		return nil, fmt.Errorf("dashboard v2 requires inspect and control handlers")
	}
	if logger == nil {
		logger = log.New(os.Stderr, "[dash] ", log.LstdFlags)
	}
	return &V2Server{
		Inspect: inspectAPI,
		Control: controlAPI,
		APIBase: "/api",
		Logger:  logger,
	}, nil
}

// Handler returns the v2 mux.
func (s *V2Server) Handler() http.Handler {
	base := strings.TrimRight(s.APIBase, "/")
	if base == "" {
		base = "/api"
	}

	mux := http.NewServeMux()
	mux.Handle(base+"/inspect/", http.StripPrefix(base+"/inspect", s.Inspect))
	mux.Handle(base+"/control/", http.StripPrefix(base+"/control", s.Control))
	if s.Vault != nil {
		mux.Handle(base+"/vault/", http.StripPrefix(base+"/vault", s.Vault))
	}

	// /dash → layout shell + pages.
	mux.Handle("GET /dash", http.RedirectHandler("/dash/", http.StatusMovedPermanently))
	mux.Handle("GET /dash/assets/", http.StripPrefix("/dash/assets", assetsHandler{}))
	// dashAPIAddr proxies selected inspect reads under the dashboard origin so
	// the browser never crosses ports. Read-only GET proxy; no mutation routes.
	mux.Handle("GET /dash/api/", http.StripPrefix("/dash/api", s.Inspect))
	mux.Handle("GET /dash/", http.HandlerFunc(s.handleOverview))
	mux.Handle("GET /dash/events", http.HandlerFunc(s.handleEvents))
	mux.Handle("GET /dash/models", http.HandlerFunc(s.handleModels))
	mux.Handle("GET /dash/resources", http.HandlerFunc(s.handleResources))
	return mux
}

func (s *V2Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.Overview()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render overview: %v", err)
	}
}

// handleEvents serves the dedicated events explorer page with filters,
// pagination and live tail fragment.
func (s *V2Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.Events()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render events: %v", err)
	}
}

// handleModels serves the model bindings and context-pressure posture page.
func (s *V2Server) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.Models()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render models: %v", err)
	}
}

// handleResources serves the ResourceGate posture page fed by /resources.
func (s *V2Server) handleResources(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.Resources()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render resources: %v", err)
	}
}
