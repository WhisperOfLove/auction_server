package service

import (
	"log"
	"strconv"
	"strings"
	"time"

	"auction_server/internal/appsettings"
	"auction_server/internal/cache"
	"auction_server/internal/config"
	"auction_server/internal/domain"
	"auction_server/internal/events"
	"auction_server/internal/repository"
	"auction_server/internal/ws"
)

type AuctionService struct {
	repo     repository.AuctionRepository
	hub      *ws.Hub
	settings appsettings.Provider
	cfg      config.Config
}

func NewAuctionService(repo repository.AuctionRepository, hub *ws.Hub, settings appsettings.Provider, cfg config.Config) *AuctionService {
	return &AuctionService{
		repo:     repo,
		hub:      hub,
		settings: settings,
		cfg:      cfg,
	}
}

func (s *AuctionService) resolved() appsettings.Settings {
	if s.settings != nil {
		return appsettings.Resolved(s.settings.Get(), s.cfg)
	}
	return appsettings.DefaultsFromConfig(s.cfg)
}

func (s *AuctionService) bidCooldownMs() int64 {
	sec := s.resolved().BidCooldownSeconds
	if sec < 0 {
		sec = 0
	}
	return int64(sec) * 1000
}

func (s *AuctionService) ListAuctions(filter domain.AuctionFeedFilter) []domain.Auction {
	return s.repo.ListAuctions(filter)
}

func (s *AuctionService) ListFeedPage(filter domain.AuctionFeedFilter) domain.AuctionFeedPage {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	key := cache.FeedCacheKey(string(filter.Status), filter.Sort, filter.Cursor, filter.Limit, filter.BidderUserID)
	if page, ok := cache.GetFeedPage(key); ok {
		return *page
	}
	page := s.repo.ListAuctionsPage(filter)
	cache.SetFeedPage(key, page)
	return page
}

func (s *AuctionService) ListMyAuctions(ownerID string, status domain.AuctionStatus) []domain.Auction {
	return s.repo.ListOwnerAuctions(ownerID, status)
}

func (s *AuctionService) PostPeriodCounts() []domain.AdminPeriodCount {
	type postCounter interface {
		PostPeriodCounts() []domain.AdminPeriodCount
	}
	if repo, ok := s.repo.(postCounter); ok {
		return repo.PostPeriodCounts()
	}
	return []domain.AdminPeriodCount{
		{Label: "روز", Value: 0},
		{Label: "هفته", Value: 0},
		{Label: "ماه", Value: 0},
	}
}

func (s *AuctionService) GetAuction(id string) (domain.Auction, bool) {
	return s.repo.GetAuctionByID(id)
}

func (s *AuctionService) CreateAuction(input domain.CreateAuctionInput) (domain.Auction, bool) {
	if strings.TrimSpace(input.OwnerID) == "" ||
		strings.TrimSpace(input.Title) == "" ||
		strings.TrimSpace(input.Description) == "" {
		return domain.Auction{}, false
	}
	if input.DurationHours <= 0 {
		input.DurationHours = s.resolved().PostDurationHours
	}
	return s.repo.CreateAuction(input), true
}

func (s *AuctionService) TopBids(auctionID string) []domain.Bid {
	if bids, ok := cache.GetTopBids(auctionID); ok {
		return bids
	}
	bids := s.repo.TopBids(auctionID, 5)
	cache.SetTopBids(auctionID, bids)
	return bids
}

func (s *AuctionService) PlaceBid(auctionID string, input domain.PlaceBidInput) (domain.Bid, bool, string) {
	if strings.TrimSpace(input.UserName) == "" || input.Price <= 0 {
		return domain.Bid{}, false, "userName and positive price are required"
	}
	if strings.TrimSpace(input.UserID) == "" {
		input.UserID = input.Phone
	}
	if strings.TrimSpace(input.UserID) == "" {
		input.UserID = input.UserName
	}
	if strings.TrimSpace(input.Phone) == "" {
		input.Phone = "0000000000"
	}
	now := time.Now().UnixMilli()
	cdMs := s.bidCooldownMs()
	if cdMs > 0 {
		lastBidAt, okLast := s.repo.LastBidAtMs(auctionID, input.UserID)
		if okLast && lastBidAt > 0 {
			nextAllowedAt := lastBidAt + cdMs
			if nextAllowedAt > now {
				remainSec := (nextAllowedAt - now + 999) / 1000
				return domain.Bid{}, false, "برای ثبت پیشنهاد بعدی " + strconv.FormatInt(remainSec, 10) + " ثانیه صبر کنید"
			}
		}
	}
	if a, ok := s.repo.GetAuctionByID(auctionID); !ok {
		log.Printf("[bid] auction missing id=%s user=%s price=%d", auctionID, input.UserID, input.Price)
		return domain.Bid{}, false, "حراج پیدا نشد"
	} else if a.ModerationStatus != domain.ModerationApproved {
		log.Printf("[bid] not approved id=%s moderation=%s user=%s", auctionID, a.ModerationStatus, input.UserID)
		return domain.Bid{}, false, "این حراج هنوز منتشر نشده است"
	} else if a.Status != domain.AuctionStatusActive {
		log.Printf("[bid] ended id=%s status=%s user=%s", auctionID, a.Status, input.UserID)
		return domain.Bid{}, false, "این حراج پایان یافته است"
	} else if a.ExpiresAtMs > 0 && a.ExpiresAtMs <= now {
		log.Printf("[bid] expired id=%s expires=%d user=%s", auctionID, a.ExpiresAtMs, input.UserID)
		return domain.Bid{}, false, "زمان این حراج به پایان رسیده است"
	}

	minRequired, okReq := s.repo.MinRequiredBid(auctionID)
	if !okReq {
		log.Printf("[bid] min_required failed id=%s user=%s price=%d", auctionID, input.UserID, input.Price)
		return domain.Bid{}, false, "حراج پیدا نشد"
	}
	if input.Price < minRequired {
		log.Printf("[bid] too low id=%s user=%s price=%d need>=%d", auctionID, input.UserID, input.Price, minRequired)
		return domain.Bid{}, false, "مبلغ پیشنهادی شما کمتر از حداقل مجاز است"
	}
	bid, ok := s.repo.PlaceBid(auctionID, input)
	if !ok {
		log.Printf("[bid] place failed id=%s user=%s price=%d min=%d", auctionID, input.UserID, input.Price, minRequired)
		return domain.Bid{}, false, "ثبت پیشنهاد ممکن نیست (حراج پایان یافته یا خطای سرور)"
	}
	bids := s.repo.TopBids(auctionID, 5)
	cache.SetTopBids(auctionID, bids)
	if s.hub != nil {
		s.hub.NotifyTopBids(auctionID)
	}
	events.PublishBidPlaced(events.BidPlacedEvent{
		AuctionID: auctionID,
		UserID:    bid.UserID,
		Price:     bid.Price,
		AtMs:      bid.CreatedAt,
	})
	return bid, true, ""
}

func (s *AuctionService) BidCooldown(auctionID, userID string) domain.BidCooldown {
	now := time.Now().UnixMilli()
	cdMs := s.bidCooldownMs()
	out := domain.BidCooldown{
		CanBid:      true,
		CooldownMs:  cdMs,
		NextBidAtMs: 0,
		RemainingMs: 0,
		LastBidAtMs: 0,
	}
	if strings.TrimSpace(auctionID) == "" || strings.TrimSpace(userID) == "" || cdMs <= 0 {
		return out
	}
	lastBidAt, ok := s.repo.LastBidAtMs(auctionID, userID)
	if !ok || lastBidAt <= 0 {
		return out
	}
	nextAllowedAt := lastBidAt + cdMs
	out.LastBidAtMs = lastBidAt
	out.NextBidAtMs = nextAllowedAt
	if nextAllowedAt > now {
		out.CanBid = false
		out.RemainingMs = nextAllowedAt - now
	}
	return out
}

func (s *AuctionService) DeleteBid(auctionID, userID string) bool {
	if strings.TrimSpace(auctionID) == "" || strings.TrimSpace(userID) == "" {
		return false
	}
	ok := s.repo.DeleteBid(auctionID, userID)
	if ok {
		cache.InvalidateTopBids(auctionID)
		// Feed pages may include offers_count; safe to let TTL expire or redeploy clears Redis.
		if s.hub != nil {
			s.hub.NotifyTopBids(auctionID)
		}
	}
	return ok
}

func (s *AuctionService) ResultContacts(auctionID, ownerID string) ([]domain.BidSummary, bool) {
	return s.repo.ResultContacts(auctionID, ownerID, 20)
}

func (s *AuctionService) ExtendAuction(auctionID, ownerID string, hours int) bool {
	return s.repo.ExtendAuction(auctionID, ownerID, hours)
}

func (s *AuctionService) BumpAuction(auctionID, ownerID string) bool {
	return s.repo.BumpAuction(auctionID, ownerID)
}

func (s *AuctionService) SetFeatured(auctionID, ownerID string) bool {
	return s.repo.SetFeatured(auctionID, ownerID)
}

func (s *AuctionService) DeleteAuction(auctionID, ownerID string) bool {
	return s.repo.DeleteAuction(auctionID, ownerID)
}

func (s *AuctionService) UpdateAuction(auctionID string, input domain.UpdateAuctionInput) (domain.Auction, bool) {
	if strings.TrimSpace(input.OwnerID) == "" ||
		strings.TrimSpace(input.Title) == "" ||
		strings.TrimSpace(input.Description) == "" {
		return domain.Auction{}, false
	}
	return s.repo.UpdateAuction(auctionID, input)
}

func (s *AuctionService) AuctionStats(auctionID, ownerID string) (domain.AuctionStats, bool) {
	return s.repo.AuctionStats(auctionID, ownerID)
}
