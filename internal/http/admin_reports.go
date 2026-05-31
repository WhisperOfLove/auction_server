package http

import (
	"net/http"

	"auction_server/internal/domain"
)

func (s *Server) handleAdminReportsOverview(w http.ResponseWriter, r *http.Request) {
	if !s.adminCORS(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	out := domain.AdminReportsResponse{
		Posts: s.auctions.PostPeriodCounts(),
	}
	if s.users != nil {
		out.Registrations = s.users.RegistrationPeriodCounts()
	}
	writeJSON(w, http.StatusOK, out)
}
