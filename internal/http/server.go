package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"auction_server/internal/appsettings"
	"auction_server/internal/cache"
	"auction_server/internal/config"
	"auction_server/internal/domain"
	"auction_server/internal/repository"
	"auction_server/internal/repository/postgres"
	"auction_server/internal/service"
	"auction_server/internal/ws"
)

type Server struct {
	cfg        config.Config
	settings   appsettings.Provider
	mux        *http.ServeMux
	auctions   *service.AuctionService
	moderation *service.ModerationService
	users      *postgres.UserRepository
	chat       *postgres.ChatRepository
	hub        *ws.Hub
}

func NewServer(cfg config.Config, repo repository.AuctionRepository, users *postgres.UserRepository, chat *postgres.ChatRepository, hub *ws.Hub, moderation *service.ModerationService, settings appsettings.Provider) *http.Server {
	s := &Server{
		cfg:        cfg,
		settings:   settings,
		mux:        http.NewServeMux(),
		auctions:   service.NewAuctionService(repo, hub, settings, cfg),
		moderation: moderation,
		hub:        hub,
	}
	s.routes(hub)
	s.registerUploadRoutes()
	s.registerUserChatRoutes(users, chat)
	s.registerAdminRoutes()
	s.registerNotificationRoutes()
	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: s.mux,
	}
}

func (s *Server) routes(hub *ws.Hub) {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/v1/config/realtime", s.handleRealtimeConfig)
	s.mux.HandleFunc("/v1/config/app", s.handleAppConfig)
	if hub != nil {
		s.mux.Handle("/v1/ws/", hub)
	}
	s.mux.HandleFunc("/v1/auctions", s.handleAuctions)
	s.mux.HandleFunc("/v1/auctions/", s.handleAuctionByPath)
	s.mux.HandleFunc("/v1/me/auctions", s.handleMyAuctions)
	s.mux.HandleFunc("/v1/me/auctions/", s.handleMyAuctionByPath)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"service": "auction_server",
	})
}

func (s *Server) handleRealtimeConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	base := s.effectivePublicBaseURL()
	wsURL := ""
	if base != "" {
		wsURL = strings.Replace(base, "https://", "wss://", 1)
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
		wsURL += "/v1/ws/auctions/"
	}
	_ = json.NewEncoder(w).Encode(map[string]string{
		"topBidsPushIntervalSeconds": s.cfg.TopBidsPushIntervalSeconds,
		"feedRefreshIntervalSeconds": s.cfg.FeedRefreshIntervalSeconds,
		"wsBaseUrl":                  wsURL,
	})
}

func (s *Server) handleAuctions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := parseAuctionStatus(r.URL.Query().Get("status"))
		sortBy := r.URL.Query().Get("sort")
		limitRaw := r.URL.Query().Get("limit")
		if limitRaw != "" {
			limit, _ := strconv.Atoi(limitRaw)
			cursor := r.URL.Query().Get("cursor")
			writeJSON(w, http.StatusOK, s.auctions.ListFeedPage(domain.AuctionFeedFilter{
				Status:       status,
				Sort:         sortBy,
				Limit:        limit,
				Cursor:       cursor,
				BidderUserID: strings.TrimSpace(r.URL.Query().Get("bidderUserId")),
			}))
			return
		}
		writeJSON(w, http.StatusOK, s.auctions.ListAuctions(domain.AuctionFeedFilter{
			Status: status,
			Sort:   sortBy,
		}))
	case http.MethodPost:
		var input domain.CreateAuctionInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		item, ok := s.auctions.CreateAuction(input)
		if !ok {
			writeError(w, http.StatusBadRequest, "title and description are required")
			return
		}
		if s.moderation != nil {
			s.moderation.OnAuctionSubmitted(item)
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAuctionByPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/auctions/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		writeError(w, http.StatusBadRequest, "invalid auction id")
		return
	}
	auctionID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		auction, ok := s.auctions.GetAuction(auctionID)
		if !ok {
			writeError(w, http.StatusNotFound, "auction not found")
			return
		}
		viewerID := strings.TrimSpace(r.URL.Query().Get("viewerId"))
		if auction.ModerationStatus != domain.ModerationApproved && auction.OwnerID != viewerID {
			writeError(w, http.StatusNotFound, "auction not found")
			return
		}
		writeJSON(w, http.StatusOK, auction)
		return
	}
	if len(parts) == 2 && parts[1] == "top-bids" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, s.auctions.TopBids(auctionID))
		return
	}
	if len(parts) == 2 && parts[1] == "bid-cooldown" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID := strings.TrimSpace(r.URL.Query().Get("userId"))
		if userID == "" {
			writeError(w, http.StatusBadRequest, "userId is required")
			return
		}
		writeJSON(w, http.StatusOK, s.auctions.BidCooldown(auctionID, userID))
		return
	}
	if len(parts) == 3 && parts[1] == "chat" && parts[2] == "messages" {
		s.handleAuctionChat(w, r, auctionID)
		return
	}
	if len(parts) == 2 && parts[1] == "bids" {
		switch r.Method {
		case http.MethodPost:
			var input domain.PlaceBidInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid body")
				return
			}
			bid, ok, msg := s.auctions.PlaceBid(auctionID, input)
			if !ok {
				if msg != "" {
					writeError(w, http.StatusBadRequest, msg)
				} else {
					writeError(w, http.StatusBadRequest, "invalid bid or auction")
				}
				return
			}
			writeJSON(w, http.StatusCreated, bid)
		case http.MethodDelete:
			userID := strings.TrimSpace(r.URL.Query().Get("userId"))
			if userID == "" {
				writeError(w, http.StatusBadRequest, "userId is required")
				return
			}
			if !s.auctions.DeleteBid(auctionID, userID) {
				writeError(w, http.StatusNotFound, "bid not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) handleMyAuctions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ownerID := strings.TrimSpace(r.URL.Query().Get("ownerId"))
	if ownerID == "" {
		writeError(w, http.StatusBadRequest, "ownerId is required")
		return
	}
	status := parseAuctionStatus(r.URL.Query().Get("status"))
	writeJSON(w, http.StatusOK, s.auctions.ListMyAuctions(ownerID, status))
}

func (s *Server) handleMyAuctionByPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/me/auctions/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || strings.TrimSpace(parts[0]) == "" {
		writeError(w, http.StatusBadRequest, "invalid auction id")
		return
	}
	auctionID := strings.TrimSpace(parts[0])
	ownerID := strings.TrimSpace(r.URL.Query().Get("ownerId"))
	if ownerID == "" {
		ownerID = strings.TrimSpace(r.Header.Get("X-Owner-Id"))
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch, http.MethodPut:
			var input domain.UpdateAuctionInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid body")
				return
			}
			if ownerID != "" {
				input.OwnerID = ownerID
			}
			item, ok := s.auctions.UpdateAuction(auctionID, input)
			if !ok {
				writeError(w, http.StatusForbidden, "update failed")
				return
			}
			if s.moderation != nil {
				s.moderation.OnAuctionUpdated(item)
			}
			cache.InvalidateFeedCache()
			writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if ownerID == "" {
				writeError(w, http.StatusBadRequest, "ownerId is required")
				return
			}
			if !s.auctions.DeleteAuction(auctionID, ownerID) {
				writeError(w, http.StatusForbidden, "delete failed")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	action := parts[1]
	switch action {
	case "result-contacts":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if auctionID == "" || ownerID == "" {
			writeError(w, http.StatusBadRequest, "auctionId and ownerId are required")
			return
		}
		contacts, ok := s.auctions.ResultContacts(auctionID, ownerID)
		if !ok {
			writeError(w, http.StatusForbidden, "not allowed or auction not ended")
			return
		}
		writeJSON(w, http.StatusOK, contacts)
	case "extend":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if ownerID == "" {
			writeError(w, http.StatusBadRequest, "ownerId is required")
			return
		}
		if !s.auctions.ExtendAuction(auctionID, ownerID, s.resolvedSettings().ExtendDurationHours) {
			writeError(w, http.StatusForbidden, "extend failed")
			return
		}
		item, ok := s.auctions.GetAuction(auctionID)
		if !ok {
			writeError(w, http.StatusNotFound, "auction not found")
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "bump":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if ownerID == "" {
			writeError(w, http.StatusBadRequest, "ownerId is required")
			return
		}
		if !s.auctions.BumpAuction(auctionID, ownerID) {
			writeError(w, http.StatusForbidden, "bump failed")
			return
		}
		cache.InvalidateFeedCache()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case "feature":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if ownerID == "" {
			writeError(w, http.StatusBadRequest, "ownerId is required")
			return
		}
		if !s.auctions.SetFeatured(auctionID, ownerID) {
			writeError(w, http.StatusForbidden, "feature failed")
			return
		}
		cache.InvalidateFeedCache()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case "stats":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if ownerID == "" {
			writeError(w, http.StatusBadRequest, "ownerId is required")
			return
		}
		stats, ok := s.auctions.AuctionStats(auctionID, ownerID)
		if !ok {
			writeError(w, http.StatusForbidden, "not allowed")
			return
		}
		writeJSON(w, http.StatusOK, stats)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func parseAuctionStatus(value string) domain.AuctionStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(domain.AuctionStatusActive):
		return domain.AuctionStatusActive
	case string(domain.AuctionStatusEnded):
		return domain.AuctionStatusEnded
	case string(domain.AuctionStatusArchived):
		return domain.AuctionStatusArchived
	default:
		return ""
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
