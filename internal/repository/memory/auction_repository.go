package memory

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"auction_server/internal/domain"
)

type AuctionRepository struct {
	mu       sync.RWMutex
	auctions map[string]domain.Auction
	bids     map[string][]domain.Bid
	nextID   int64
}

func NewAuctionRepository() *AuctionRepository {
	now := time.Now().UnixMilli()
	r := &AuctionRepository{
		auctions: map[string]domain.Auction{},
		bids:     map[string][]domain.Bid{},
		nextID:   3,
	}
	a1 := domain.Auction{
		ID:          "auction-1",
		OwnerID:     "user-1",
		Title:       "ساعت مچی کلاسیک",
		Description: "ساعت سالم و کم‌کارکرد",
		ImageURLs:   []string{"https://example.com/auction-1.jpg"},
		Status:      domain.AuctionStatusActive,
		CreatedAtMs: now - 120000,
		ExpiresAtMs: now + (24 * 60 * 60 * 1000),
	}
	a2 := domain.Auction{
		ID:          "auction-2",
		OwnerID:     "user-1",
		Title:       "گوشی موبایل کارکرده",
		Description: "گوشی تمیز با باتری سالم",
		ImageURLs:   []string{"https://example.com/auction-2.jpg"},
		Status:      domain.AuctionStatusEnded,
		CreatedAtMs: now - (26 * 60 * 60 * 1000),
		ExpiresAtMs: now - (2 * 60 * 60 * 1000),
		EndedAtMs:   now - (2 * 60 * 60 * 1000),
	}
	r.auctions[a1.ID] = a1
	r.auctions[a2.ID] = a2
	r.bids[a1.ID] = []domain.Bid{
		{AuctionID: a1.ID, UserName: "کاربر ۱", Price: 231_000_000, CreatedAt: now - 60000},
		{AuctionID: a1.ID, UserName: "کاربر ۲", Price: 228_000_000, CreatedAt: now - 40000},
	}
	r.bids[a2.ID] = []domain.Bid{
		{AuctionID: a2.ID, UserName: "کاربر ۳", Price: 140_000_000, CreatedAt: now - 50000},
	}
	r.refreshAuctionAggregatesLocked(a1.ID)
	r.refreshAuctionAggregatesLocked(a2.ID)
	return r
}

func (r *AuctionRepository) ListAuctions(filter domain.AuctionFeedFilter) []domain.Auction {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(time.Now().UnixMilli())
	result := make([]domain.Auction, 0, len(r.auctions))
	for _, a := range r.auctions {
		if filter.Status != "" && a.Status != filter.Status {
			continue
		}
		result = append(result, a)
	}
	if filter.Sort == "trending" {
		sort.Slice(result, func(i, j int) bool {
			if result[i].OffersCount == result[j].OffersCount {
				return result[i].CreatedAtMs > result[j].CreatedAtMs
			}
			return result[i].OffersCount > result[j].OffersCount
		})
	} else {
		sort.Slice(result, func(i, j int) bool {
			return result[i].CreatedAtMs > result[j].CreatedAtMs
		})
	}
	return result
}

func (r *AuctionRepository) ListOwnerAuctions(ownerID string, status domain.AuctionStatus) []domain.Auction {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(time.Now().UnixMilli())
	result := make([]domain.Auction, 0)
	for _, a := range r.auctions {
		if a.OwnerID != ownerID {
			continue
		}
		if status != "" && a.Status != status {
			continue
		}
		result = append(result, a)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAtMs > result[j].CreatedAtMs
	})
	return result
}

func (r *AuctionRepository) GetAuctionByID(id string) (domain.Auction, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(time.Now().UnixMilli())
	a, ok := r.auctions[id]
	return a, ok
}

func (r *AuctionRepository) CreateAuction(input domain.CreateAuctionInput) domain.Auction {
	r.mu.Lock()
	defer r.mu.Unlock()
	durationHours := input.DurationHours
	if durationHours <= 0 {
		durationHours = 24
	}
	now := time.Now().UnixMilli()
	id := fmt.Sprintf("auction-%d", r.nextID)
	r.nextID++
	a := domain.Auction{
		ID:          id,
		OwnerID:     input.OwnerID,
		Title:       input.Title,
		Description: input.Description,
		ImageURLs:   input.ImageURLs,
		Status:      domain.AuctionStatusActive,
		CreatedAtMs: now,
		ExpiresAtMs: now + int64(durationHours)*60*60*1000,
	}
	r.auctions[id] = a
	r.bids[id] = []domain.Bid{}
	return a
}

func (r *AuctionRepository) TopBids(auctionID string, limit int) []domain.Bid {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(time.Now().UnixMilli())
	list := append([]domain.Bid{}, r.bids[auctionID]...)
	sort.Slice(list, func(i, j int) bool {
		return list[i].Price > list[j].Price
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list
}

func (r *AuctionRepository) PlaceBid(auctionID string, input domain.PlaceBidInput) (domain.Bid, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(time.Now().UnixMilli())
	auction, ok := r.auctions[auctionID]
	if !ok || auction.Status != domain.AuctionStatusActive {
		return domain.Bid{}, false
	}
	bid := domain.Bid{
		AuctionID: auctionID,
		UserName:  input.UserName,
		Price:     input.Price,
		CreatedAt: time.Now().UnixMilli(),
	}
	r.bids[auctionID] = append(r.bids[auctionID], bid)
	r.refreshAuctionAggregatesLocked(auctionID)
	return bid, true
}

func (r *AuctionRepository) ResultContacts(auctionID, ownerID string, limit int) ([]domain.BidSummary, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(time.Now().UnixMilli())
	auction, ok := r.auctions[auctionID]
	if !ok || auction.OwnerID != ownerID || auction.Status != domain.AuctionStatusEnded {
		return nil, false
	}
	out := append([]domain.BidSummary{}, auction.FinalTopOffers...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, true
}

func (r *AuctionRepository) FinalizeExpired(nowMillis int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(nowMillis)
}

func (r *AuctionRepository) finalizeExpiredLocked(nowMillis int64) {
	for id, auction := range r.auctions {
		if auction.Status == domain.AuctionStatusActive && auction.ExpiresAtMs <= nowMillis {
			auction.Status = domain.AuctionStatusEnded
			auction.EndedAtMs = nowMillis
			r.auctions[id] = auction
			r.refreshAuctionAggregatesLocked(id)
		}
	}
}

func (r *AuctionRepository) refreshAuctionAggregatesLocked(auctionID string) {
	auction, ok := r.auctions[auctionID]
	if !ok {
		return
	}
	list := append([]domain.Bid{}, r.bids[auctionID]...)
	sort.Slice(list, func(i, j int) bool {
		return list[i].Price > list[j].Price
	})
	auction.OffersCount = len(list)
	top := make([]domain.BidSummary, 0, 5)
	for i, bid := range list {
		if i >= 5 {
			break
		}
		top = append(top, domain.BidSummary{
			UserName: bid.UserName,
			Price:    bid.Price,
		})
	}
	auction.FinalTopOffers = top
	r.auctions[auctionID] = auction
}
