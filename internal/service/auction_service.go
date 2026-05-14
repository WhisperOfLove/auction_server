package service

import (
	"strings"
	"time"

	"auction_server/internal/domain"
	"auction_server/internal/repository"
)

type AuctionService struct {
	repo repository.AuctionRepository
}

func NewAuctionService(repo repository.AuctionRepository) *AuctionService {
	return &AuctionService{repo: repo}
}

func (s *AuctionService) ListAuctions(filter domain.AuctionFeedFilter) []domain.Auction {
	s.repo.FinalizeExpired(time.Now().UnixMilli())
	return s.repo.ListAuctions(filter)
}

func (s *AuctionService) ListMyAuctions(ownerID string, status domain.AuctionStatus) []domain.Auction {
	s.repo.FinalizeExpired(time.Now().UnixMilli())
	return s.repo.ListOwnerAuctions(ownerID, status)
}

func (s *AuctionService) GetAuction(id string) (domain.Auction, bool) {
	s.repo.FinalizeExpired(time.Now().UnixMilli())
	return s.repo.GetAuctionByID(id)
}

func (s *AuctionService) CreateAuction(input domain.CreateAuctionInput) (domain.Auction, bool) {
	if strings.TrimSpace(input.OwnerID) == "" ||
		strings.TrimSpace(input.Title) == "" ||
		strings.TrimSpace(input.Description) == "" {
		return domain.Auction{}, false
	}
	if input.DurationHours <= 0 {
		input.DurationHours = 24
	}
	return s.repo.CreateAuction(input), true
}

func (s *AuctionService) TopBids(auctionID string) []domain.Bid {
	s.repo.FinalizeExpired(time.Now().UnixMilli())
	return s.repo.TopBids(auctionID, 5)
}

func (s *AuctionService) PlaceBid(auctionID string, input domain.PlaceBidInput) (domain.Bid, bool) {
	if strings.TrimSpace(input.UserName) == "" || input.Price <= 0 {
		return domain.Bid{}, false
	}
	s.repo.FinalizeExpired(time.Now().UnixMilli())
	return s.repo.PlaceBid(auctionID, input)
}

func (s *AuctionService) ResultContacts(auctionID, ownerID string) ([]domain.BidSummary, bool) {
	s.repo.FinalizeExpired(time.Now().UnixMilli())
	return s.repo.ResultContacts(auctionID, ownerID, 20)
}
