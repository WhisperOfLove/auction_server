package memory

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"auction_server/internal/domain"
)

func (r *AuctionRepository) CountModerationPending() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.countPendingLocked()
}

func (r *AuctionRepository) countPendingLocked() int {
	n := 0
	for _, a := range r.auctions {
		if a.ModerationStatus == domain.ModerationPending {
			n++
		}
	}
	return n
}

func (r *AuctionRepository) ListModerationQueue(filter domain.ModerationQueueFilter) domain.ModerationQueuePage {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := filter.Status
	if status == "" {
		status = domain.ModerationPending
	}
	list := make([]domain.Auction, 0)
	for _, a := range r.auctions {
		if a.ModerationStatus == status {
			list = append(list, a)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAtMs == list[j].CreatedAtMs {
			return list[i].ID > list[j].ID
		}
		return list[i].CreatedAtMs > list[j].CreatedAtMs
	})
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	start := 0
	if filter.Cursor != "" {
		for i, a := range list {
			key := fmt.Sprintf("%d:%s", a.CreatedAtMs, a.ID)
			if key == filter.Cursor || a.ID == filter.Cursor {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	hasMore := end < len(list)
	if end > len(list) {
		end = len(list)
	}
	page := list[start:end]
	var next string
	if hasMore && len(page) > 0 {
		last := page[len(page)-1]
		next = fmt.Sprintf("%d:%s", last.CreatedAtMs, last.ID)
	}
	return domain.ModerationQueuePage{
		Items:        page,
		NextCursor:   next,
		HasMore:      hasMore,
		PendingCount: r.countPendingLocked(),
	}
}

func (r *AuctionRepository) ApproveModeration(id string) (domain.Auction, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.auctions[id]
	if !ok || a.ModerationStatus != domain.ModerationPending {
		return domain.Auction{}, false
	}
	now := time.Now().UnixMilli()
	durationHours := int64(72)
	if a.ExpiresAtMs > a.CreatedAtMs {
		durationHours = (a.ExpiresAtMs - a.CreatedAtMs) / (3600 * 1000)
		if durationHours <= 0 {
			durationHours = 72
		}
	}
	a.ModerationStatus = domain.ModerationApproved
	a.ModerationReason = ""
	a.CreatedAtMs = now
	a.BumpedAtMs = now
	a.ExpiresAtMs = now + durationHours*3600*1000
	a.Status = domain.AuctionStatusActive
	r.auctions[id] = a
	return a, true
}

func (r *AuctionRepository) RejectModeration(id, reason string) (domain.Auction, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.auctions[id]
	if !ok || a.ModerationStatus != domain.ModerationPending {
		return domain.Auction{}, false
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "مطابق قوانین حراج نیست."
	}
	a.ModerationStatus = domain.ModerationRejected
	a.ModerationReason = reason
	a.IsFeatured = false
	r.auctions[id] = a
	return a, true
}
