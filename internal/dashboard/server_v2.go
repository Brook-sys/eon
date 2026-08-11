package dashboard

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"motor-autonomo/internal/dashboard/views"
)

// V2Server serves the Templ-based dashboard pages under /.
// It handles UI routes and acts as a router for /api/* requests.
type V2Server struct {
	// Inspect/Control are the same handlers used by the existing dashboard.
	Inspect http.Handler
	Control http.Handler
	// Vault is an optional localhost-only write-only credential surface.
	Vault http.Handler
	// APIBase is the browser-visible prefix for fetch/EventSource calls.
	APIBase string
	// DefaultMissionID sets the initial mission ID loaded by the UI.
	DefaultMissionID string
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

	// / -> layout shell + pages.
	mux.Handle("GET /{$}", http.RedirectHandler("/dash/", http.StatusMovedPermanently))
	mux.Handle("GET /dash/{$}", http.HandlerFunc(s.handleOverview))

	mux.Handle("GET /assets/", http.StripPrefix("/assets", assetsHandler{}))

	// Base API mappings remain

	// Add explicit mapping for views
	mux.Handle("GET /events", http.HandlerFunc(s.handleEvents))
	mux.Handle("GET /events/", http.HandlerFunc(s.handleEventDetail))
	mux.Handle("GET /models", http.HandlerFunc(s.handleModels))
	mux.Handle("GET /resources", http.HandlerFunc(s.handleResources))
	mux.Handle("GET /frontier", http.HandlerFunc(s.handleFrontier))
	mux.Handle("GET /alerts", http.HandlerFunc(s.handleAlerts))
	mux.Handle("GET /knowledge", http.HandlerFunc(s.handleKnowledge))
	mux.Handle("GET /partials/overview", http.HandlerFunc(s.handlePartialOverview))

	// Keep backwards compatibility for a moment, or redirect. Let's just point them to same handlers.
	mux.Handle("GET /dash/assets/", http.StripPrefix("/dash/assets", assetsHandler{}))
	// Proxy inspect, control and vault endpoints under /dash/api/ so browser calls work seamlessly
	mux.Handle("/dash/api/control/", http.StripPrefix("/dash/api/control", s.Control))
	if s.Vault != nil {
		mux.Handle("/dash/api/vault/", http.StripPrefix("/dash/api/vault", s.Vault))
	}
	mux.Handle("/dash/api/", http.StripPrefix("/dash/api", s.Inspect))
	mux.Handle("GET /dash/events", http.HandlerFunc(s.handleEvents))
	mux.Handle("GET /dash/events/", http.HandlerFunc(s.handleEventDetail))
	mux.Handle("GET /dash/models", http.HandlerFunc(s.handleModels))
	mux.Handle("GET /dash/resources", http.HandlerFunc(s.handleResources))
	mux.Handle("GET /dash/frontier", http.HandlerFunc(s.handleFrontier))
	mux.Handle("GET /dash/alerts", http.HandlerFunc(s.handleAlerts))
	mux.Handle("GET /dash/knowledge", http.HandlerFunc(s.handleKnowledge))
	mux.Handle("GET /dash/partials/overview", http.HandlerFunc(s.handlePartialOverview))
	return mux
}

func (s *V2Server) handlePartialOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.PartialOverview()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render partial overview: %v", err)
	}
}

func (s *V2Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.Overview()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render overview: %v", err)
	}
}

func (s *V2Server) handleEventDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	id := strings.TrimPrefix(r.URL.Path, "/dash/events/")
	if id == "" {
		http.Redirect(w, r, "/dash/events", http.StatusFound)
		return
	}
	component := views.EventDetail(id)
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render event detail: %v", err)
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

// handleFrontier serves the work-opportunity browse page fed by /frontier.
func (s *V2Server) handleFrontier(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.Frontier()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render frontier: %v", err)
	}
}

// handleAlerts serves the dedicated alerts page fed by /alerts.
func (s *V2Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.Alerts()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render alerts: %v", err)
	}
}

// handleKnowledge serves the knowledge catalog page fed by /knowledge endpoints.
func (s *V2Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	component := views.Knowledge()
	if err := component.Render(r.Context(), w); err != nil {
		s.Logger.Printf("render knowledge: %v", err)
	}
}
