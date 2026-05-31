package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"auction_server/internal/domain"
)

func (s *Server) handleAdminTicketThreads(w http.ResponseWriter, r *http.Request) {
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
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not available")
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	writeJSON(w, http.StatusOK, s.chat.ListSupportThreads(search, limit))
}

func (s *Server) handleAdminTicketMessages(w http.ResponseWriter, r *http.Request) {
	if !s.adminCORS(w, r) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not available")
		return
	}
	switch r.Method {
	case http.MethodGet:
		userID := strings.TrimSpace(r.URL.Query().Get("userId"))
		if userID == "" {
			writeError(w, http.StatusBadRequest, "userId is required")
			return
		}
		writeJSON(w, http.StatusOK, s.chat.ListMessages(domain.SupportAuctionID, domain.SupportAdminID, userID, 300))
	case http.MethodPost:
		var input domain.AdminTicketMessageInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		userID := strings.TrimSpace(input.UserID)
		body := strings.TrimSpace(input.Body)
		attachment := strings.TrimSpace(input.AttachmentURL)
		if userID == "" || (body == "" && attachment == "") {
			writeError(w, http.StatusBadRequest, "userId and body or attachmentUrl are required")
			return
		}
		msg, ok := s.chat.InsertMessage(domain.SupportAuctionID, domain.SendChatInput{
			SenderID:      domain.SupportAdminID,
			PeerID:        userID,
			SenderName:    domain.SupportAdminID,
			Body:          body,
			AttachmentURL: attachment,
		})
		if !ok {
			writeError(w, http.StatusBadRequest, "could not send message")
			return
		}
		if s.hub != nil {
			s.hub.NotifyChat(domain.SupportAuctionID, domain.SupportAdminID, userID)
		}
		writeJSON(w, http.StatusCreated, msg)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
