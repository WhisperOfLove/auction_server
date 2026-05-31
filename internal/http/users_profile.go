package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"auction_server/internal/cache"
	"auction_server/internal/domain"
	"auction_server/internal/repository/postgres"
)

func (s *Server) registerUserProfileRoutes() {
	if s.users == nil {
		return
	}
	s.mux.HandleFunc("/v1/me/profile", s.handleMeProfile)
	s.mux.HandleFunc("/v1/me/bid-auctions", s.handleMeBidAuctions)
	s.mux.HandleFunc("/v1/users/", s.handleUserSocialPath)
}

func (s *Server) handleMeBidAuctions(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		writeError(w, http.StatusServiceUnavailable, "users not available")
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
	ids := s.users.BidAuctionIDs(userID)
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, ids)
}

func (s *Server) handleMeProfile(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		writeError(w, http.StatusServiceUnavailable, "users not available")
		return
	}
	switch r.Method {
	case http.MethodGet:
		userID := strings.TrimSpace(r.URL.Query().Get("userId"))
		if userID == "" {
			writeError(w, http.StatusBadRequest, "userId is required")
			return
		}
		u, ok := s.users.GetByID(userID)
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, u)
	case http.MethodPatch, http.MethodPut:
		var input domain.ProfileUpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		input.UserID = strings.TrimSpace(input.UserID)
		if input.UserID == "" {
			writeError(w, http.StatusBadRequest, "userId is required")
			return
		}
		u, err := s.users.UpdateProfile(input)
		if err != nil {
			if errors.Is(err, postgres.ErrDisplayNameTaken) {
				writeError(w, http.StatusConflict, "امکان استفاده از این نام نمایشی وجود ندارد")
				return
			}
			writeError(w, http.StatusBadRequest, "could not update profile")
			return
		}
		if input.ReceiveCalls != nil {
			for _, auctionID := range s.users.BidAuctionIDs(input.UserID) {
				cache.InvalidateTopBids(auctionID)
			}
		}
		writeJSON(w, http.StatusOK, u)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// publicUserView hides phone when the user disabled incoming calls.
func publicUserView(u domain.User) domain.User {
	if !u.ReceiveCalls {
		u.Phone = ""
	}
	return u
}

func (s *Server) handleUserSocialPath(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		writeError(w, http.StatusServiceUnavailable, "users not available")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/users/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	userID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		u, ok := s.users.GetByID(userID)
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, publicUserView(u))
		return
	}
	if len(parts) == 2 && parts[1] == "profile-page" && r.Method == http.MethodGet {
		u, ok := s.users.GetByID(userID)
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		now := time.Now().UnixMilli()
		viewerID := strings.TrimSpace(r.URL.Query().Get("viewerId"))
		raw := s.auctions.ListMyAuctions(userID, domain.AuctionStatusActive)
		active := make([]domain.Auction, 0, len(raw))
		for _, a := range raw {
			if a.ModerationStatus != domain.ModerationApproved && viewerID != userID {
				continue
			}
			if a.ExpiresAtMs <= 0 || a.ExpiresAtMs > now || a.ModerationStatus == domain.ModerationPending {
				active = append(active, a)
			}
		}
		writeJSON(w, http.StatusOK, domain.PublicProfilePage{
			User:          publicUserView(u),
			FollowerCount: s.users.CountFollowers(userID),
			ActivePosts:   active,
		})
		return
	}
	if len(parts) == 2 && parts[1] == "follow" {
		viewerID := strings.TrimSpace(r.URL.Query().Get("viewerId"))
		switch r.Method {
		case http.MethodPost:
			if viewerID == "" {
				writeError(w, http.StatusBadRequest, "viewerId is required")
				return
			}
			if !s.users.Follow(viewerID, userID) {
				writeError(w, http.StatusBadRequest, "could not follow")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		case http.MethodDelete:
			if viewerID == "" {
				writeError(w, http.StatusBadRequest, "viewerId is required")
				return
			}
			s.users.Unfollow(viewerID, userID)
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]bool{"following": s.users.IsFollowing(viewerID, userID)})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) == 2 && parts[1] == "following" && r.Method == http.MethodGet {
		viewerID := strings.TrimSpace(r.URL.Query().Get("viewerId"))
		if viewerID == "" || viewerID != userID {
			writeError(w, http.StatusBadRequest, "viewerId must match user")
			return
		}
		list := s.users.ListFollowing(userID, 50)
		if list == nil {
			list = []domain.FollowUserSummary{}
		}
		writeJSON(w, http.StatusOK, list)
		return
	}
	if len(parts) == 2 && parts[1] == "block" {
		viewerID := strings.TrimSpace(r.URL.Query().Get("viewerId"))
		switch r.Method {
		case http.MethodPost:
			if viewerID == "" {
				writeError(w, http.StatusBadRequest, "viewerId is required")
				return
			}
			if !s.users.Block(viewerID, userID) {
				writeError(w, http.StatusBadRequest, "could not block")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		case http.MethodDelete:
			if viewerID == "" {
				writeError(w, http.StatusBadRequest, "viewerId is required")
				return
			}
			s.users.Unblock(viewerID, userID)
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		case http.MethodGet:
			if viewerID == "" {
				writeError(w, http.StatusBadRequest, "viewerId is required")
				return
			}
			writeJSON(w, http.StatusOK, s.users.BlockStatus(viewerID, userID))
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) == 2 && parts[1] == "blocked" && r.Method == http.MethodGet {
		viewerID := strings.TrimSpace(r.URL.Query().Get("viewerId"))
		if viewerID == "" || viewerID != userID {
			writeError(w, http.StatusBadRequest, "viewerId must match user")
			return
		}
		list := s.users.ListBlocked(userID, 50)
		if list == nil {
			list = []domain.BlockUserSummary{}
		}
		writeJSON(w, http.StatusOK, list)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}
