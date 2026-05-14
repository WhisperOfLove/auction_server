package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"auction_server/internal/config"
	"auction_server/internal/domain"
	"auction_server/internal/repository/memory"
	"auction_server/internal/service"
)

type Server struct {
	cfg      config.Config
	mux      *http.ServeMux
	auctions *service.AuctionService
}

func NewServer(cfg config.Config) *http.Server {
	repo := memory.NewAuctionRepository()
	s := &Server{
		cfg:      cfg,
		mux:      http.NewServeMux(),
		auctions: service.NewAuctionService(repo),
	}
	s.routes()
	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: s.mux,
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/v1/config/realtime", s.handleRealtimeConfig)
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
	_ = json.NewEncoder(w).Encode(map[string]string{
		"topBidsPushIntervalSeconds": s.cfg.TopBidsPushIntervalSeconds,
		"feedRefreshIntervalSeconds": s.cfg.FeedRefreshIntervalSeconds,
	})
}

func (s *Server) handleAuctions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := parseAuctionStatus(r.URL.Query().Get("status"))
		sortBy := r.URL.Query().Get("sort")
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
	if len(parts) == 2 && parts[1] == "bids" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input domain.PlaceBidInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		bid, ok := s.auctions.PlaceBid(auctionID, input)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid bid or auction")
			return
		}
		writeJSON(w, http.StatusCreated, bid)
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
	if len(parts) != 2 || parts[1] != "result-contacts" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	auctionID := strings.TrimSpace(parts[0])
	ownerID := strings.TrimSpace(r.URL.Query().Get("ownerId"))
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
