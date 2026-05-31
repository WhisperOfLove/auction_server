package service

import (
	"log"
	"strings"

	"auction_server/internal/cache"
	"auction_server/internal/domain"
	"auction_server/internal/events"
	"auction_server/internal/moderation"
	"auction_server/internal/repository"
	"auction_server/internal/repository/postgres"
	"auction_server/internal/ws"
)

type ModerationService struct {
	auctions repository.AuctionRepository
	notifs   *postgres.NotificationRepository
	chat     *postgres.ChatRepository
	hub      *ws.Hub
}

func NewModerationService(
	auctions repository.AuctionRepository,
	notifs *postgres.NotificationRepository,
	chat *postgres.ChatRepository,
	hub *ws.Hub,
) *ModerationService {
	return &ModerationService{auctions: auctions, notifs: notifs, chat: chat, hub: hub}
}

// OnAuctionSubmitted queues an auction for admin review (new post or owner edit).
func (s *ModerationService) OnAuctionSubmitted(a domain.Auction) {
	moderation.EnqueuePending(a.ID, a.CreatedAtMs)
	events.PublishAuctionSubmitted(events.AuctionSubmittedEvent{
		AuctionID: a.ID,
		OwnerID:   a.OwnerID,
		Title:     a.Title,
		AtMs:      a.CreatedAtMs,
	})
}

// OnAuctionUpdated reuses the same admin queue path as a new submission.
func (s *ModerationService) OnAuctionUpdated(a domain.Auction) {
	s.OnAuctionSubmitted(a)
}

func (s *ModerationService) ListQueue(filter domain.ModerationQueueFilter) domain.ModerationQueuePage {
	page := s.auctions.ListModerationQueue(filter)
	if n := moderation.PendingCount(); n >= 0 {
		page.PendingCount = n
	}
	return page
}

func (s *ModerationService) Approve(id string) (domain.Auction, bool) {
	log.Printf("moderation_approve_start auction_id=%s", id)
	a, ok := s.auctions.ApproveModeration(id)
	if !ok {
		log.Printf("moderation_approve_fail auction_id=%s reason=not_pending_or_missing", id)
		return domain.Auction{}, false
	}
	moderation.DequeuePending(id)
	cache.InvalidateFeedCache()
	log.Printf("moderation_approve_done auction_id=%s owner_id=%s status=%s moderation_status=%s", a.ID, a.OwnerID, a.Status, a.ModerationStatus)
	events.PublishModerationDecided(events.ModerationDecidedEvent{
		AuctionID: id,
		OwnerID:   a.OwnerID,
		Decision:  string(domain.ModerationApproved),
		AtMs:      a.CreatedAtMs,
	})
	return a, true
}

func (s *ModerationService) Reject(id, reason string) (domain.Auction, bool) {
	log.Printf("moderation_reject_start auction_id=%s", id)
	a, ok := s.auctions.RejectModeration(id, reason)
	if !ok {
		log.Printf("moderation_reject_fail auction_id=%s reason=not_pending_or_missing", id)
		return domain.Auction{}, false
	}
	moderation.DequeuePending(id)
	reason = strings.TrimSpace(a.ModerationReason)
	createdChat := false
	if s.chat != nil {
		body := "حراج شما رد شد"
		if a.Title != "" {
			body += " («" + strings.TrimSpace(a.Title) + "»)"
		}
		if reason != "" {
			body += ": " + reason
		}
		_, ok := s.chat.InsertMessage(domain.SupportAuctionID, domain.SendChatInput{
			SenderID:   domain.SupportAdminID,
			PeerID:     a.OwnerID,
			SenderName: domain.SupportAdminID,
			Body:       body,
		})
		createdChat = ok
		if !ok {
			log.Printf("moderation_reject_chat_fail auction_id=%s owner_id=%s", a.ID, a.OwnerID)
		}
		if s.hub != nil {
			s.hub.NotifyChat(domain.SupportAuctionID, domain.SupportAdminID, a.OwnerID)
		}
	}
	log.Printf("moderation_reject_done auction_id=%s owner_id=%s status=%s moderation_status=%s chat_created=%t", a.ID, a.OwnerID, a.Status, a.ModerationStatus, createdChat)
	events.PublishModerationDecided(events.ModerationDecidedEvent{
		AuctionID: id,
		OwnerID:   a.OwnerID,
		Decision:  string(domain.ModerationRejected),
		Reason:    reason,
		AtMs:      a.CreatedAtMs,
	})
	return a, true
}

func (s *ModerationService) ListNotifications(userID string, limit int) []domain.UserNotification {
	if s.notifs == nil {
		return nil
	}
	return s.notifs.ListForUser(userID, limit)
}

func (s *ModerationService) MarkNotificationRead(userID string, id int64) bool {
	if s.notifs == nil {
		return false
	}
	return s.notifs.MarkRead(userID, id)
}
