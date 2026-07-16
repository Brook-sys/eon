// Package dashboard serves the experimental operator web UI.
//
// The UI is a thin browser surface over the Control API: it reads projections
// and submits typed answers/commands. It never writes the canonical store
// directly and is not required for mission continuity.
package dashboard

import (
	"errors"
	"net/http"
	"strings"
)

// Server hosts the static operator dashboard and reverse-mounts API trees.
// InspectAPI and ControlAPI are expected to already be root-relative handlers
// (e.g. inspect routes at /health, control routes at /commands).
type Server struct {
	Inspect http.Handler
	Control http.Handler
	// APIBase is the browser-visible prefix for fetch/EventSource calls.
	// Default "/api" mounts inspect under /api/inspect and control under /api/control.
	APIBase string
	// DefaultMissionID is pre-filled in the UI when set (local single-mission runs).
	DefaultMissionID string
}

func New(inspectAPI, controlAPI http.Handler) (*Server, error) {
	if inspectAPI == nil || controlAPI == nil {
		return nil, errors.New("dashboard requires inspect and control handlers")
	}
	return &Server{
		Inspect: inspectAPI,
		Control: controlAPI,
		APIBase: "/api",
	}, nil
}

// Handler returns the combined dashboard + API mux.
func (s *Server) Handler() http.Handler {
	base := strings.TrimRight(s.APIBase, "/")
	if base == "" {
		base = "/api"
	}
	mux := http.NewServeMux()
	mux.Handle(base+"/inspect/", http.StripPrefix(base+"/inspect", s.Inspect))
	mux.Handle(base+"/control/", http.StripPrefix(base+"/control", s.Control))
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /dashboard", s.handleIndex)
	mux.HandleFunc("GET /dashboard/", s.handleIndex)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(renderIndex(s.APIBase, s.DefaultMissionID)))
}
