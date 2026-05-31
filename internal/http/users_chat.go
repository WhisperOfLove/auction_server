package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"auction_server/internal/domain"
	"auction_server/internal/repository/postgres"
)

func (s *Server) registerUserChatRoutes(users *postgres.UserRepository, chat *postgres.ChatRepository) {
	s.users = users
	s.chat = chat
	if users != nil {
		s.mux.HandleFunc("/v1/auth/login", s.handleAuthLogin)
		s.mux.HandleFunc("/v1/users", s.handleUsers)
		s.registerUserProfileRoutes()
	}
	if chat != nil {
		s.mux.HandleFunc("/v1/me/chat/inbox", s.handleChatInbox)
		s.mux.HandleFunc("/v1/chat/report", s.handleChatReport)
		s.mux.HandleFunc("/v1/me/support/messages", s.handleSupportMessages)
	}
}

func (s *Server) handleChatInbox(w http.ResponseWriter, r *http.Request) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not available")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	threads := s.chat.ListInbox(userID, 50)
	if threads == nil {
		threads = []domain.ChatThread{}
	}
	writeJSON(w, http.StatusOK, threads)
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		writeError(w, http.StatusServiceUnavailable, "auth not available")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input domain.LoginInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	user, ok := s.users.Login(input.Username, input.Password)
	if !ok {
		writeError(w, http.StatusUnauthorized, "نام کاربری یا رمز عبور اشتباه است")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		writeError(w, http.StatusServiceUnavailable, "users store not available")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input domain.UpsertUserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	input.ID = strings.TrimSpace(input.ID)
	input.Phone = strings.TrimSpace(input.Phone)
	if input.Phone == "" {
		writeError(w, http.StatusBadRequest, "phone is required")
		return
	}
	user, ok := s.users.UpsertUser(input)
	if !ok {
		writeError(w, http.StatusInternalServerError, "could not save user")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleAuctionChat(w http.ResponseWriter, r *http.Request, auctionID string) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not available")
		return
	}
	switch r.Method {
	case http.MethodGet:
		viewerID := strings.TrimSpace(r.URL.Query().Get("viewerId"))
		peerID := strings.TrimSpace(r.URL.Query().Get("peerId"))
		writeJSON(w, http.StatusOK, s.chat.ListMessages(auctionID, viewerID, peerID, 200))
	case http.MethodPost:
		var input domain.SendChatInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		input.SenderID = strings.TrimSpace(input.SenderID)
		input.PeerID = strings.TrimSpace(input.PeerID)
		input.SenderName = strings.TrimSpace(input.SenderName)
		input.Body = strings.TrimSpace(input.Body)
		input.AttachmentURL = strings.TrimSpace(input.AttachmentURL)
		if input.SenderID == "" || input.PeerID == "" || (input.Body == "" && input.AttachmentURL == "") {
			writeError(w, http.StatusBadRequest, "senderId, peerId and body or attachmentUrl are required")
			return
		}
		if input.SenderName == "" {
			input.SenderName = input.SenderID
		}
		if s.users != nil {
			st := s.users.BlockStatus(input.SenderID, input.PeerID)
			if !st.CanChat {
				writeError(w, http.StatusForbidden, "blocked")
				return
			}
		}
		msg, ok := s.chat.InsertMessage(auctionID, input)
		if !ok {
			writeError(w, http.StatusBadRequest, "could not send message")
			return
		}
		if s.hub != nil {
			s.hub.NotifyChat(auctionID, input.SenderID, input.PeerID)
		}
		writeJSON(w, http.StatusCreated, msg)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleChatReport(w http.ResponseWriter, r *http.Request) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not available")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input domain.ChatReportInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	input.AuctionID = strings.TrimSpace(input.AuctionID)
	input.ReporterID = strings.TrimSpace(input.ReporterID)
	input.PeerID = strings.TrimSpace(input.PeerID)
	input.Body = strings.TrimSpace(input.Body)
	if input.AuctionID == "" || input.ReporterID == "" || input.PeerID == "" {
		writeError(w, http.StatusBadRequest, "auctionId, reporterId and peerId are required")
		return
	}
	if !s.chat.ReportChat(input) {
		writeError(w, http.StatusInternalServerError, "could not save report")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (s *Server) handleSupportMessages(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusOK, s.chat.ListMessages(domain.SupportAuctionID, userID, domain.SupportAdminID, 300))
	case http.MethodPost:
		var input domain.SendChatInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		input.SenderID = strings.TrimSpace(input.SenderID)
		input.Body = strings.TrimSpace(input.Body)
		input.AttachmentURL = strings.TrimSpace(input.AttachmentURL)
		if input.SenderID == "" || (input.Body == "" && input.AttachmentURL == "") {
			writeError(w, http.StatusBadRequest, "senderId and body or attachmentUrl are required")
			return
		}
		input.PeerID = domain.SupportAdminID
		if strings.TrimSpace(input.SenderName) == "" {
			input.SenderName = input.SenderID
		}
		msg, ok := s.chat.InsertMessage(domain.SupportAuctionID, input)
		if !ok {
			writeError(w, http.StatusBadRequest, "could not send message")
			return
		}
		if s.hub != nil {
			s.hub.NotifyChat(domain.SupportAuctionID, input.SenderID, domain.SupportAdminID)
		}
		writeJSON(w, http.StatusCreated, msg)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
