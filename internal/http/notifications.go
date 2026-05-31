package http

import (
	"net/http"
	"strconv"
	"strings"

	"auction_server/internal/domain"
)

func (s *Server) registerNotificationRoutes() {
	if s.moderation == nil {
		return
	}
	s.mux.HandleFunc("/v1/me/notifications", s.handleMyNotifications)
	s.mux.HandleFunc("/v1/me/notifications/", s.handleMyNotificationByID)
}

func (s *Server) handleMyNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list := s.moderation.ListNotifications(userID, limit)
	if list == nil {
		list = []domain.UserNotification{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleMyNotificationByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/me/notifications/")
	id, err := strconv.ParseInt(strings.Trim(path, "/"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid notification id")
		return
	}
	if !s.moderation.MarkNotificationRead(userID, id) {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
