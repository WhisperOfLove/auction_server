package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"auction_server/internal/domain"
)

func (s *Server) registerAdminRoutes() {
	if s.moderation == nil {
		return
	}
	s.mux.HandleFunc("/v1/admin/moderation/queue", s.handleAdminModerationQueue)
	s.mux.HandleFunc("/v1/admin/moderation/auctions/", s.handleAdminModerationAuction)
	s.mux.HandleFunc("/v1/admin/tickets/threads", s.handleAdminTicketThreads)
	s.mux.HandleFunc("/v1/admin/tickets/messages", s.handleAdminTicketMessages)
	s.mux.HandleFunc("/v1/admin/users", s.handleAdminUsers)
	s.mux.HandleFunc("/v1/admin/reports/overview", s.handleAdminReportsOverview)
	s.mux.HandleFunc("/v1/admin/app-settings", s.handleAdminAppSettings)
}

func (s *Server) adminCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "X-Admin-Key, Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	key := strings.TrimSpace(s.cfg.AdminAPIKey)
	if key == "" {
		writeError(w, http.StatusServiceUnavailable, "admin API disabled")
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if got == "" {
		got = strings.TrimSpace(r.URL.Query().Get("adminKey"))
	}
	if got != key {
		writeError(w, http.StatusUnauthorized, "invalid admin key")
		return false
	}
	return true
}

func (s *Server) handleAdminModerationQueue(w http.ResponseWriter, r *http.Request) {
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
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	status := domain.ModerationStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = domain.ModerationPending
	}
	writeJSON(w, http.StatusOK, s.moderation.ListQueue(domain.ModerationQueueFilter{
		Status: status,
		Limit:  limit,
		Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
	}))
}

func (s *Server) handleAdminModerationAuction(w http.ResponseWriter, r *http.Request) {
	if !s.adminCORS(w, r) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	// Expected path:
	//   /v1/admin/moderation/auctions/{auctionId}/{approve|reject}
	// Parse from the end to avoid any prefix/trailing-slash issues.
	clean := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(clean, "/")
	if len(parts) < 3 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if parts[len(parts)-3] != "auctions" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	id := parts[len(parts)-2]
	action := parts[len(parts)-1]
	log.Printf("moderation_admin_request action=%s auction_id=%s method=%s", action, id, r.Method)
	switch action {
	case "approve":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a, ok := s.moderation.Approve(id)
		if !ok {
			log.Printf("moderation_admin_result action=approve auction_id=%s ok=false reason=auction_not_pending", id)
			writeError(w, http.StatusNotFound, "auction not pending")
			return
		}
		log.Printf("moderation_admin_result action=approve auction_id=%s ok=true owner_id=%s moderation_status=%s", a.ID, a.OwnerID, a.ModerationStatus)
		writeJSON(w, http.StatusOK, a)
	case "reject":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body domain.ModerationDecisionInput
		_ = json.NewDecoder(r.Body).Decode(&body)
		a, ok := s.moderation.Reject(id, body.Reason)
		if !ok {
			log.Printf("moderation_admin_result action=reject auction_id=%s ok=false reason=auction_not_pending", id)
			writeError(w, http.StatusNotFound, "auction not pending")
			return
		}
		log.Printf("moderation_admin_result action=reject auction_id=%s ok=true owner_id=%s moderation_status=%s", a.ID, a.OwnerID, a.ModerationStatus)
		writeJSON(w, http.StatusOK, a)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}
