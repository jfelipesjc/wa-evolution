package api

import (
	"embed"
	"net/http"
)

// managerUI holds the single-page Manager UI (vanilla HTML/JS, no build step, no
// external dependencies) embedded into the binary so the "one binary, zero deps"
// deployment story holds: the dashboard ships with the server.
//
//go:embed manager_ui/index.html
var managerUI embed.FS

// handleManager serves the Manager dashboard at /manager. It is exempt from the
// apikey middleware (see authMiddleware) because the page itself is public; the
// browser-side API calls it makes carry the apikey the operator pastes on login.
func (s *Server) handleManager(w http.ResponseWriter, r *http.Request) {
	page, err := managerUI.ReadFile("manager_ui/index.html")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "manager UI unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(page)
}
