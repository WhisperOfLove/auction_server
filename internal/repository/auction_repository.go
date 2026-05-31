package repository

import "auction_server/internal/domain"

type AuctionRepository interface {
	ListAuctions(filter domain.AuctionFeedFilter) []domain.Auction
	ListAuctionsPage(filter domain.AuctionFeedFilter) domain.AuctionFeedPage
	ListOwnerAuctions(ownerID string, status domain.AuctionStatus) []domain.Auction
	GetAuctionByID(id string) (domain.Auction, bool)
	CreateAuction(input domain.CreateAuctionInput) domain.Auction
	TopBids(auctionID string, limit int) []domain.Bid
	LastBidAtMs(auctionID, userID string) (int64, bool)
	MinRequiredBid(auctionID string) (int64, bool)
	PlaceBid(auctionID string, input domain.PlaceBidInput) (domain.Bid, bool)
	DeleteBid(auctionID, userID string) bool
	ResultContacts(auctionID, ownerID string, limit int) ([]domain.BidSummary, bool)
	ExtendAuction(auctionID, ownerID string, hours int) bool
	BumpAuction(auctionID, ownerID string) bool
	SetFeatured(auctionID, ownerID string) bool
	DeleteAuction(auctionID, ownerID string) bool
	UpdateAuction(auctionID string, input domain.UpdateAuctionInput) (domain.Auction, bool)
	AuctionStats(auctionID, ownerID string) (domain.AuctionStats, bool)
	FinalizeExpired(nowMillis int64)

	// Moderation queue (admin approval before public feed).
	ListModerationQueue(filter domain.ModerationQueueFilter) domain.ModerationQueuePage
	CountModerationPending() int
	ApproveModeration(id string) (domain.Auction, bool)
	RejectModeration(id, reason string) (domain.Auction, bool)
}
