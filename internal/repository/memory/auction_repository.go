package memory

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"auction_server/internal/domain"
)

type AuctionRepository struct {
	mu                    sync.RWMutex
	auctions              map[string]domain.Auction
	bids                  map[string][]domain.Bid
	bidCooldownAt         map[string]map[string]int64
	nextID                int64
	deleteDaysAfterExpiry int
}

func NewAuctionRepository(deleteDaysAfterExpiry int) *AuctionRepository {
	if deleteDaysAfterExpiry < 0 {
		deleteDaysAfterExpiry = 0
	}
	return &AuctionRepository{
		auctions:              map[string]domain.Auction{},
		bids:                  map[string][]domain.Bid{},
		bidCooldownAt:         map[string]map[string]int64{},
		nextID:                1,
		deleteDaysAfterExpiry: deleteDaysAfterExpiry,
	}
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
		if filter.Status == domain.AuctionStatusActive && a.ModerationStatus != domain.ModerationApproved {
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
			if sortKeyMs(result[i]) != sortKeyMs(result[j]) {
				return sortKeyMs(result[i]) > sortKeyMs(result[j])
			}
			return result[i].ID > result[j].ID
		})
	}
	return result
}

func (r *AuctionRepository) userBidAuctionSet(userID string) map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := make(map[string]bool)
	for auctionID, list := range r.bids {
		for _, b := range list {
			if b.UserID == userID {
				set[auctionID] = true
				break
			}
		}
	}
	return set
}

func (r *AuctionRepository) ListAuctionsPage(filter domain.AuctionFeedFilter) domain.AuctionFeedPage {
	all := r.ListAuctions(filter)
	if bidder := strings.TrimSpace(filter.BidderUserID); bidder != "" {
		allowed := r.userBidAuctionSet(bidder)
		filtered := make([]domain.Auction, 0, len(all))
		for _, a := range all {
			if allowed[a.ID] {
				filtered = append(filtered, a)
			}
		}
		all = filtered
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	start := 0
	if filter.Cursor != "" {
		for i, a := range all {
			if a.ID == filter.Cursor || fmt.Sprintf("%d:%s", sortKeyMs(a), a.ID) == filter.Cursor {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	hasMore := end < len(all)
	if end > len(all) {
		end = len(all)
	}
	items := all[start:end]
	next := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		next = fmt.Sprintf("%d:%s", sortKeyMs(last), last.ID)
	}
	return domain.AuctionFeedPage{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}
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
	if !ok {
		return domain.Auction{}, false
	}
	a.TopBids = r.topBidsLocked(id, 5)
	return a, ok
}

func (r *AuctionRepository) topBidsLocked(auctionID string, limit int) []domain.Bid {
	list := append([]domain.Bid{}, r.bids[auctionID]...)
	sort.Slice(list, func(i, j int) bool {
		if list[i].Price == list[j].Price {
			return list[i].CreatedAt < list[j].CreatedAt
		}
		return list[i].Price > list[j].Price
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list
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
		ID:               id,
		OwnerID:          input.OwnerID,
		Title:            input.Title,
		Description:      input.Description,
		ImageURLs:        input.ImageURLs,
		Status:           domain.AuctionStatusActive,
		ModerationStatus: domain.ModerationPending,
		CreatedAtMs:      now,
		ExpiresAtMs:      now + int64(durationHours)*60*60*1000,
		BasePrice:        input.BasePrice,
		BumpedAtMs:       now,
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

func (r *AuctionRepository) LastBidAtMs(auctionID, userID string) (int64, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if strings.TrimSpace(auctionID) == "" || strings.TrimSpace(userID) == "" {
		return 0, false
	}
	if byUser, ok := r.bidCooldownAt[auctionID]; ok {
		if at, ok := byUser[userID]; ok && at > 0 {
			return at, true
		}
	}
	var maxAt int64
	for _, b := range r.bids[auctionID] {
		if b.UserID == userID && b.CreatedAt > maxAt {
			maxAt = b.CreatedAt
		}
	}
	return maxAt, true
}

func (r *AuctionRepository) MinRequiredBid(auctionID string) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(time.Now().UnixMilli())
	auction, ok := r.auctions[auctionID]
	if !ok || auction.Status != domain.AuctionStatusActive {
		return 0, false
	}
	list := r.bids[auctionID]
	if len(list) == 0 {
		if auction.BasePrice > 0 {
			return auction.BasePrice, true
		}
		return 1, true
	}
	var top int64
	for _, b := range list {
		if b.Price > top {
			top = b.Price
		}
	}
	step := auction.MinBidStep
	if step <= 0 && len(list) > 0 {
		first := list[0]
		for _, b := range list[1:] {
			if b.CreatedAt < first.CreatedAt {
				first = b
			}
		}
		step = roundBidStepMemory(first.Price / 100)
		if step < 1 {
			step = 1
		}
		auction.MinBidStep = step
		r.auctions[auctionID] = auction
	}
	if step <= 0 {
		return 0, false
	}
	return top + step, true
}

func roundBidStepMemory(onePercent int64) int64 {
	if onePercent < 1 {
		return 1
	}
	if onePercent < 1000 {
		return onePercent
	}
	return (onePercent / 1000) * 1000
}

func (r *AuctionRepository) PlaceBid(auctionID string, input domain.PlaceBidInput) (domain.Bid, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeExpiredLocked(time.Now().UnixMilli())
	auction, ok := r.auctions[auctionID]
	if !ok || auction.Status != domain.AuctionStatusActive {
		return domain.Bid{}, false
	}
	userID := strings.TrimSpace(input.UserID)
	now := time.Now().UnixMilli()
	bid := domain.Bid{
		AuctionID: auctionID,
		UserID:    userID,
		UserName:  input.UserName,
		Phone:     input.Phone,
		Price:     input.Price,
		CreatedAt: now,
	}
	list := r.bids[auctionID]
	replaced := false
	for i, b := range list {
		if b.UserID == userID && userID != "" {
			list[i] = bid
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, bid)
		if auction.MinBidStep == 0 {
			step := roundBidStepMemory(input.Price / 100)
			auction.MinBidStep = step
			r.auctions[auctionID] = auction
		}
	}
	r.bids[auctionID] = list
	if r.bidCooldownAt == nil {
		r.bidCooldownAt = make(map[string]map[string]int64)
	}
	if r.bidCooldownAt[auctionID] == nil {
		r.bidCooldownAt[auctionID] = make(map[string]int64)
	}
	r.bidCooldownAt[auctionID][userID] = now
	r.refreshAuctionAggregatesLocked(auctionID)
	return bid, true
}

func (r *AuctionRepository) DeleteBid(auctionID, userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	auctionID = strings.TrimSpace(auctionID)
	userID = strings.TrimSpace(userID)
	if auctionID == "" || userID == "" {
		return false
	}
	list := r.bids[auctionID]
	out := list[:0]
	removed := false
	for _, b := range list {
		if b.UserID == userID {
			removed = true
			continue
		}
		out = append(out, b)
	}
	if !removed {
		return false
	}
	r.bids[auctionID] = out
	r.refreshAuctionAggregatesLocked(auctionID)
	return true
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
	expiryDeleteCutoff := int64(0)
	if r.deleteDaysAfterExpiry > 0 {
		expiryDeleteCutoff = nowMillis - int64(r.deleteDaysAfterExpiry)*24*60*60*1000
	}
	for id, auction := range r.auctions {
		if auction.Status == domain.AuctionStatusActive && auction.ExpiresAtMs > 0 && auction.ExpiresAtMs <= nowMillis {
			auction.Status = domain.AuctionStatusEnded
			auction.EndedAtMs = nowMillis
			r.auctions[id] = auction
			r.refreshAuctionAggregatesLocked(id)
		}
		if r.deleteDaysAfterExpiry > 0 &&
			auction.Status == domain.AuctionStatusEnded &&
			auction.EndedAtMs > 0 &&
			auction.EndedAtMs <= expiryDeleteCutoff {
			delete(r.auctions, id)
			delete(r.bids, id)
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
	unique := make(map[string]struct{})
	for _, bid := range list {
		unique[bid.UserName] = struct{}{}
	}
	auction.OffersCount = len(unique)
	top := make([]domain.BidSummary, 0, 5)
	for i, bid := range list {
		if i >= 5 {
			break
		}
		top = append(top, domain.BidSummary{
			UserName: bid.UserName,
			Phone:    bid.Phone,
			Price:    bid.Price,
		})
	}
	auction.FinalTopOffers = top
	r.auctions[auctionID] = auction
}

func (r *AuctionRepository) ExtendAuction(auctionID, ownerID string, hours int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.auctions[auctionID]
	if !ok || a.OwnerID != ownerID {
		return false
	}
	now := time.Now().UnixMilli()
	add := int64(hours) * 60 * 60 * 1000
	if a.ExpiresAtMs < now {
		a.ExpiresAtMs = now
	}
	a.ExpiresAtMs += add
	a.Status = domain.AuctionStatusActive
	a.EndedAtMs = 0
	r.auctions[auctionID] = a
	return true
}

func (r *AuctionRepository) BumpAuction(auctionID, ownerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.auctions[auctionID]
	if !ok || a.OwnerID != ownerID || a.Status != domain.AuctionStatusActive ||
		a.ModerationStatus != domain.ModerationApproved {
		return false
	}
	a.BumpedAtMs = time.Now().UnixMilli()
	r.auctions[auctionID] = a
	return true
}

func (r *AuctionRepository) DeleteAuction(auctionID, ownerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.auctions[auctionID]
	if !ok || a.OwnerID != ownerID {
		return false
	}
	delete(r.auctions, auctionID)
	delete(r.bids, auctionID)
	return true
}

func (r *AuctionRepository) UpdateAuction(auctionID string, input domain.UpdateAuctionInput) (domain.Auction, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.auctions[auctionID]
	if !ok || a.OwnerID != input.OwnerID {
		return domain.Auction{}, false
	}
	a.Title = input.Title
	a.Description = input.Description
	if len(input.ImageURLs) > 0 {
		a.ImageURLs = input.ImageURLs
	}
	a.BasePrice = input.BasePrice
	a.ModerationStatus = domain.ModerationPending
	a.ModerationReason = ""
	a.IsFeatured = false
	r.auctions[auctionID] = a
	return a, true
}

func (r *AuctionRepository) SetFeatured(auctionID, ownerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.auctions[auctionID]
	if !ok || a.OwnerID != ownerID || a.Status != domain.AuctionStatusActive ||
		a.ModerationStatus != domain.ModerationApproved {
		return false
	}
	now := time.Now().UnixMilli()
	a.IsFeatured = true
	a.BumpedAtMs = now
	r.auctions[auctionID] = a
	return true
}

func (r *AuctionRepository) AuctionStats(auctionID, ownerID string) (domain.AuctionStats, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.auctions[auctionID]
	if !ok || a.OwnerID != ownerID {
		return domain.AuctionStats{}, false
	}
	users := make(map[string]struct{})
	for _, b := range r.bids[auctionID] {
		if b.UserID != "" {
			users[b.UserID] = struct{}{}
		} else {
			users[b.UserName] = struct{}{}
		}
	}
	return domain.AuctionStats{
		ViewCount:      a.ViewCount,
		OfferUserCount: len(users),
		OffersCount:    len(r.bids[auctionID]),
	}, true
}

func sortKeyMs(a domain.Auction) int64 {
	if a.BumpedAtMs > 0 {
		return a.BumpedAtMs
	}
	return a.CreatedAtMs
}
