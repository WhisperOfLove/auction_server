package repository

import "auction_server/internal/domain"

type AuctionRepository interface {
	ListAuctions(filter domain.AuctionFeedFilter) []domain.Auction
	ListOwnerAuctions(ownerID string, status domain.AuctionStatus) []domain.Auction
	GetAuctionByID(id string) (domain.Auction, bool)
	CreateAuction(input domain.CreateAuctionInput) domain.Auction
	TopBids(auctionID string, limit int) []domain.Bid
	PlaceBid(auctionID string, input domain.PlaceBidInput) (domain.Bid, bool)
	ResultContacts(auctionID, ownerID string, limit int) ([]domain.BidSummary, bool)
	FinalizeExpired(nowMillis int64)
}
