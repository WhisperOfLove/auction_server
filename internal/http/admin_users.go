package http

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
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
	if s.users == nil {
		writeError(w, http.StatusServiceUnavailable, "users not available")
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	writeJSON(w, http.StatusOK, s.users.ListForAdmin(search, limit, offset))
}
